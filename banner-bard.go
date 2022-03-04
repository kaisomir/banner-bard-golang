/*
 * Banner Bard: Banner-serving discord bot, sire.
 *
 * banner-bard.go: main() and all commands go here. It's roughly
 * ordered in 3 sections:
 *   1. Utility functions
 *   2. init(), main(), and messageCreate().
 *   3. All the command functions
 *
 * This program uses the BSD 3-Clause license. You can find details under
 * the file LICENSE or under <https://opensource.org/licenses/BSD-3-Clause>.
 */
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
)

const SettingsFile = "settings.json"

// Static error messages
const GeneralError = "Sire, I've seem to be hit with troubles."
const SqlError = "Sire, the SQL server is having many troubles. " +
	"Perchance the maintainer could diagnose this probelm in our future."
const DiscordError = "I'm sorry, sire, but Discord gives us woe! " +
	"Mayhaps we find a better fortune anon when times are less dark."
const FileTypeError = "Sire, I can't find the filetype for this tag. " +
	"I need a URL that ends in jpg, jpeg, or png."

const OkMessage = "Yes, sire."
const NoActiveScheduleMessage = "Sire, I don't have any tags queued up at the moment."

var TimeUnits = map[rune]time.Duration{
	's': time.Second,
	'm': time.Minute,
	'h': time.Hour,
	'd': time.Hour * 24,
	'w': time.Hour * 24 * 7,
}

var logger = log.New(os.Stdout, "Banner Bard: ",
	log.Ldate|log.Ltime|log.Lshortfile)

var Settings struct {
	ClientID     string
	Token        string
	OwnerID      string
	AllowedRoles []string
	GuildID      string
	LogChannelID string
	Prefix       string
}

var BardEvaluator CommandEvaluator
var Scheduler *BannerScheduler

// Open the globally-set SettingsFile path and marshall the data in the global Settings struct.
func loadSettingsOrPanic() {
	f, err := os.Open(SettingsFile)
	if err != nil {
		panic(err)
	}

	decoder := json.NewDecoder(f)
	if err = decoder.Decode(&Settings); err != nil {
		panic(err)
	}
}

// Return the URL recommended to start the bot.
func botUrl() string {
	return fmt.Sprintf("https://discordapp.com/oauth2/authorize"+
		"?client_id=%s&scope=bot&permissions=3104",
		Settings.ClientID)
}

/* Generic handling of errors. If errors exist, log them all out. Return
 * whether there were errors.
 */
func handleErrors(s *discordgo.Session, channelID string,
	flavor string, source string, errs ...error) bool {
	// Yes, handling errors in Go is extremely messy, and Go doesn't have
	// many tools to abstract away Error handling -- and this messy
	// function shows it. However, handling errors by value (alongside
	// keeping the bot to do one thing well) appears to pay dividends by
	// not having any complaints in the bot code for a *loong* time.

	// Collect all errors that happened
	realErrs := []error{}
	for _, err := range errs {
		if err != nil {
			realErrs = append(realErrs, err)
			logger.Println(source + ": " + err.Error())
		}
	}

	if len(realErrs) == 0 {
		// No errors, no need to handle anything.
		return false
	}

	if channelID == "" {
		channelID = Settings.LogChannelID
	}

	buf := bytes.Buffer{}
	for _, err := range realErrs {
		buf.WriteString(err.Error() + "\n")
	}
	s.ChannelMessageSend(channelID, buf.String())
	return true
}

func handleCommandErrors(ctx *CommandContext, flavor string, errs ...error) bool {
	// Helper function to unwrap error-handling within context of
	// a command.
	return handleErrors(ctx.Session, ctx.Event.ChannelID,
		flavor, ctx.CommandName, errs...)
}

// Banner setting

/* Return the MIME subtype of a banner-allowed file by its extension, or "" if
 * not recognized. Banners allow only png and jpg, so we only check for this.
 */
func imageType(url string) string {
	url = strings.ToLower(url)
	switch {
	case strings.HasSuffix(url, "jpg"):
		return "jpg"
	case strings.HasSuffix(url, "jpeg"):
		return "jpg"
	case strings.HasSuffix(url, "png"):
		return "png"
	default:
		return ""
	}
}

/* Set the banner of the guild configured by the SettingsFile with the name of
 * the tag. An error is returned if the tag doesn't exist, the tag's URL
 * rotted, or Discord failed to set the banner.
 */
func setBanner(s *discordgo.Session, name string) error {
	tag, err := namedTag(name)
	if err != nil {
		return err
	}

	resp, err := http.Get(tag.Url)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	buf := bytes.Buffer{}
	buf.WriteString("data:image/" + imageType(tag.Url) + ";base64,")
	enc := base64.NewEncoder(base64.StdEncoding, &buf)
	io.Copy(enc, resp.Body)
	enc.Close()

	_, err = s.GuildEdit(Settings.GuildID,
		discordgo.GuildParams{Banner: buf.String()})
	if err != nil {
		return err
	}

	// Log the action
	logger.Printf("Set banner to tag %s\n", tag)
	return nil
}

func isDigit(chr rune) bool {
	return chr >= '0' && chr <= '9'
}

func isTimeUnit(chr rune) bool {
	_, ok := TimeUnits[chr]
	return ok
}

/* Return the number of seconds of a given timespec, e.g. "10m5", or 10 minutes
 * and 5 seconds (the default unit), would return 10*60 + 5 = 605. Return an
 * error on an invalid timespec.
 */
func parseTime(raw string) (time.Duration, error) {
	digits := ""
	var unit time.Duration = 1

	for _, chr := range raw {
		switch {
		case isDigit(chr):
			digits += string(chr)
		case isTimeUnit(chr):
			unit = TimeUnits[chr]
			goto parse
		default:
			return 0, errors.New("Character not digit or time unit")
		}
	}

parse:
	value, err := strconv.Atoi(digits)
	return time.Duration(value) * unit, err
}

func init() {
	// I'm putting this in init() instead of evaluating in the
	// declaration because Go gives a circular dependence
	// otherwise (BardEvaluator references cmdHelp, which in turn
	// references BardEvaluator to buld the help message).
	const PermDefault = PermRole | PermManageServer

	BardEvaluator = BuildCommandEvaluator("I switch out banners for you, sire").
		//
		Simple("help", cmdHelp, "to show a synopsis of all my commands",
			"", PermEveryone).
		//
		Group("Tags").
		Simple("new", cmdNew, "to make a new tag or replace a preexisting tag",
			"TAG URL", PermDefault).
		Simple("del", cmdDel, "to delete a preexisting tag",
			"TAG", PermDefault).
		Simple("set", cmdSet, "to set the banner to a tag",
			"TAG", PermDefault).
		Simple("shuffle", cmdShuffle, "to shuffle through multiple tags over time",
			"INTERVAL TAGS...", PermDefault).
		Simple("cycle", cmdCycle, "to cycle through ordered tags over time",
			"INTERVAL TAGS...", PermDefault).
		Simple("play", cmdPlay, "to play through tags once only over time",
			"INTERVAL TAGS...", PermDefault).
		Simple("ls", cmdLs, "to list all tags",
			"", PermEveryone).
		Simple("show", cmdShow, "to show the tag's description",
			"TAG", PermEveryone).
		//
		Group("Playlists").
		Compound("playlist", BuildCompoundCommand(PermEveryone).
			Simple("new", cmdPlaylistNew,
				"to create or replace a new playlist",
				"PLAYLIST TAGS...", PermDefault).
			Simple("add", cmdPlaylistAdd,
				"to add tags to a playlist",
				"PLAYLIST TAGS...", PermDefault).
			Simple("rm", cmdPlaylistRm,
				"to remove tags from a playlist",
				"PLAYLIST TAGS...", PermDefault).
			Simple("del", cmdPlaylistDel, "to delete a playlist",
				"PLAYLIST", PermDefault).
			Simple("shuffle", cmdPlaylistShuffle,
				"to shuffle through a playlist over time",
				"INTERVAL PLAYLIST", PermDefault).
			Simple("cycle", cmdPlaylistCycle,
				"to cycle through the playlist over time",
				"INTERVAL PLAYLIST", PermDefault).
			Simple("play", cmdPlaylistPlay,
				"to go through a playlist once only over time",
				"INTERVAL PLAYLIST", PermDefault).
			Simple("ls", cmdPlaylistLs, "to list all playlists",
				"", PermEveryone).
			Simple("show", cmdPlaylistShow, "to show the tags in a playlist",
				"PLAYLIST", PermEveryone)).
		//
		Group("Scheduler").
		Simple("stop", cmdStop, "to stop playing through the banner queue",
			"", PermDefault).
		Simple("next", cmdNext, "to skip to the next tag in the banner queue",
			"", PermDefault).
		//
		Group("Backups").
		Simple("export", cmdExport, "to upload all tags as a csv file.",
			"", PermDefault).
		Simple("import", cmdImport, "to import tags from a csv file.",
			"", PermDefault).
		//
		Done()
}

func main() {
	loadSettingsOrPanic()
	fmt.Println("Invite this bot at", botUrl())

	// Set up the SQLite database.
	err := openDb()
	if err != nil {
		panic(err)
	}
	defer closeDbOrPanic()

	discord, err := discordgo.New("Bot " + Settings.Token)
	if err != nil {
		panic(err)
	}

	discord.AddHandler(messageCreate)

	// Open websocket connection and begin listening
	if err = discord.Open(); err != nil {
		panic(err)
	}

	// Set up the banner scheduler
	Scheduler = NewScheduler(discord)
	go Scheduler.StartJob(discord)

	// Wait here until Ctrl-C or other term signal is received.
	logger.Println("Bot is now running. Press ^C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	// Close the session with dignity.
	logger.Println("Closing gracefully...")
	discord.Close()
	logger.Println("Bye!")
}

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.GuildID != Settings.GuildID {
		// Ignore all commands outside the server
		return
	}

	if m.Author.Bot || !strings.HasPrefix(m.Content, Settings.Prefix) {
		// Disregard all bot comments and non-prefixed messages
		return
	}

	evalCommand(s, m, &BardEvaluator, Settings.Prefix)
}

/// Commands

func cmdHelp(ctx *CommandContext, args []string) {
	ctx.Reply(BardEvaluator.Help(ctx))
}

// Tag Commands

func cmdNew(ctx *CommandContext, args []string) {
	if len(args) != 2 {
		ctx.SendUsage()
		return
	}

	tag, url := args[0], args[1]

	// Check that it's a good image type.
	filetype := imageType(url)
	if filetype == "" {
		ctx.Reply(FileTypeError)
		return
	}

	err := insertTag(tag, ctx.Event.Author.ID, url)
	if handleCommandErrors(ctx, SqlError, err) {
		return
	}

	// Log the action
	logger.Printf("I'll remember `%s` as %s", tag, url)

	// Send user response
	ctx.Reply(fmt.Sprintf("I'll remember tag **%s**.", tag))
}

func cmdDel(ctx *CommandContext, args []string) {
	// TODO make variadic command
	if len(args) != 1 {
		ctx.SendUsage()
		return
	}

	tag := args[0]

	// Check that the tag already exists
	exists, err := tagExists(tag)
	if handleCommandErrors(ctx, SqlError, err) {
		return
	}

	if !exists {
		ctx.Reply("Sire, I don't remember a tag named that anyways.")
		return
	}

	// Delete from the tags table and cycle list.
	err = delTag(tag)
	if handleCommandErrors(ctx, SqlError, err) {
		return
	}

	logger.Printf("Removed tag `%s`.\n", tag)

	// Send user response
	ctx.Reply(fmt.Sprintf("Removed the tag **%s**.", tag))
}

func cmdSet(ctx *CommandContext, args []string) {
	if len(args) != 1 {
		ctx.SendUsage()
		return
	}

	name := args[0]

	Scheduler.Stop()
	err := setBanner(ctx.Session, name)
	if handleCommandErrors(ctx, GeneralError, err) {
		return
	}

	// Send user response
	ctx.Reply(OkMessage)
}

// A helper function for setting up banner scheduler commands
func scheduleTags(ctx *CommandContext, timespec string, tags []string,
	picker func() BannerPicker, invalidTagsFlavor string) {

	interval, err := parseTime(timespec)
	if err != nil {
		ctx.Reply("Sire, I can't understand the time format **" +
			timespec + "**.")
		return
	}

	if interval < time.Minute*15 {
		ctx.Reply("Sire, that's a heavy burden. Please pick a time duration longer than 15 minutes.")
		return
	}

	// Add them all to the scheduler.
	ok, err := Scheduler.Set(interval, tags, picker)
	if handleCommandErrors(ctx, SqlError, err) {
		return
	} else if !ok {
		ctx.Reply(invalidTagsFlavor)
	} else {
		ctx.Reply(OkMessage)
	}
}

func cmdShuffle(ctx *CommandContext, args []string) {
	if len(args) < 2 {
		ctx.SendUsage()
		return
	}

	timespec, tags := args[0], args[1:]
	scheduleTags(ctx, timespec, tags, ScheduleShuffle,
		"Sire, I don't seem to remember at least one of those tags.")
}

func cmdCycle(ctx *CommandContext, args []string) {
	if len(args) < 2 {
		ctx.SendUsage()
		return
	}

	timespec, tags := args[0], args[1:]
	scheduleTags(ctx, timespec, tags, ScheduleCycle,
		"Sire, I don't seem to remember at least one of those tags.")
}

// Playlist Commands

func cmdPlay(ctx *CommandContext, args []string) {
	if len(args) < 2 {
		ctx.SendUsage()
		return
	}

	timespec, tags := args[0], args[1:]
	scheduleTags(ctx, timespec, tags, ScheduleOnceonly,
		"Sire, I don't seem to remember at least one of those tags.")
}

func cmdLs(ctx *CommandContext, args []string) {
	taglist, err := allTags()
	if handleCommandErrors(ctx, SqlError, err) {
		return
	}

	page := 1
	if len(args) != 0 {
		page, err = strconv.Atoi(args[0])
	}
	if page == 0 {
		ctx.Reply("Page numbers are positive, sire.")
		return
	}

	if len(taglist) == 0 {
		ctx.Reply("It doesn't look like you have any tags, sire.")
	} else {
		maxct := (page-1)*20 + 20
		if maxct > len(taglist) {
			maxct = len(taglist)
		}
		message := fmt.Sprintf("Tags %d to %d, sire:\n```", (page-1)*20+1, maxct)
		mintag := (page-1)*20 - 1
		if mintag < 0 {
			el1 := taglist[0]
			taglist = taglist[:(page-1)*20+20]
			taglist = append([]Tag{el1}, taglist...)
		} else {
			taglist = taglist[mintag : (page-1)*20+20]
		}

		for _, tag := range taglist {
			message += fmt.Sprintf("%s\n", tag.Name)
		}
		message += "```"

		ctx.Reply(message)
	}
}

func cmdShow(ctx *CommandContext, args []string) {
	if len(args) != 1 {
		ctx.SendUsage()
		return
	}

	tag, err := namedTag(args[0])
	if err != nil && err.Error() == SqlNoRows {
		ctx.Reply("Sire, I don't recall any tags named `" + args[0] + "`.")
		return
	} else if handleCommandErrors(ctx, SqlError, err) {
		return
	}

	user, err := ctx.Session.User(tag.AuthorID)
	if handleCommandErrors(ctx, GeneralError, err) {
		return
	}
	ctx.Reply(fmt.Sprintf("**%s** by %s#%s: %s",
		tag.Name, user.Username, user.Discriminator, tag.Url))
}

func cmdPlaylistNew(ctx *CommandContext, args []string) {
	if len(args) < 2 {
		ctx.SendUsage()
		return
	}

	playlist := args[0]
	tags := args[1:]

	err := editPlaylist(playlist, tags)
	if err != nil && err.Error() == SqlForeignKey {
		ctx.Reply("Sire, I don't know all those tags yet..")
		return
	} else if handleCommandErrors(ctx, SqlError, err) {
		return
	}

	ctx.Reply("I'll remember **" + playlist + "** to be those tags from now on.")
}

func cmdPlaylistAdd(ctx *CommandContext, args []string) {
	if len(args) < 2 {
		ctx.SendUsage()
		return
	}

	playlist := args[0]
	tags := args[1:]

	err := appendPlaylist(playlist, tags)
	if !handleCommandErrors(ctx, SqlError, err) {
		ctx.Reply("I'll add those tags to " + playlist + ".")
	}
}

func cmdPlaylistRm(ctx *CommandContext, args []string) {
	if len(args) < 2 {
		ctx.SendUsage()
		return
	}

	playlist := args[0]
	tags := args[1:]

	err := reducePlaylist(playlist, tags)
	if !handleCommandErrors(ctx, SqlError, err) {
		ctx.Reply("I'll remove those tags from " + playlist + ".")
	}
}

func cmdPlaylistDel(ctx *CommandContext, args []string) {
	if len(args) != 1 {
		ctx.SendUsage()
		return
	}

	playlist := args[0]
	err := clearPlaylist(playlist)
	if !handleCommandErrors(ctx, SqlError, err) {
		ctx.Reply("I'll forget about " + playlist + " from now on.")
	}
}

func cmdPlaylistShuffle(ctx *CommandContext, args []string) {
	if len(args) != 2 {
		ctx.SendUsage()
		return
	}

	timespec, playlist := args[0], args[1]

	// Grab tags
	tags, err := playlistTags(playlist)
	if handleCommandErrors(ctx, SqlError, err) {
		return
	}

	scheduleTags(ctx, timespec, tags, ScheduleShuffle,
		fmt.Sprintf("Sire, I don't remember a playlist titled **%s**.", playlist))
}

func cmdPlaylistCycle(ctx *CommandContext, args []string) {
	if len(args) != 2 {
		ctx.SendUsage()
		return
	}

	timespec, playlist := args[0], args[1]

	// Grab tags
	tags, err := playlistTags(playlist)
	if handleCommandErrors(ctx, SqlError, err) {
		return
	}

	scheduleTags(ctx, timespec, tags, ScheduleCycle,
		fmt.Sprintf("Sire, I don't remember a playlist titled **%s**.", playlist))
}

func cmdPlaylistPlay(ctx *CommandContext, args []string) {
	if len(args) != 2 {
		ctx.SendUsage()
		return
	}

	timespec, playlist := args[0], args[1]

	// Grab tags
	tags, err := playlistTags(playlist)
	if handleCommandErrors(ctx, SqlError, err) {
		return
	}

	scheduleTags(ctx, timespec, tags, ScheduleOnceonly,
		fmt.Sprintf("Sire, I don't remember a playlist titled **%s**.", playlist))
}

func cmdPlaylistLs(ctx *CommandContext, args []string) {
	playlists, err := allPlaylists()
	if handleCommandErrors(ctx, SqlError, err) {
		return
	}

	buf := bytes.Buffer{}
	buf.WriteString("Your playlists, sire:\n")
	for _, playlist := range playlists {
		buf.WriteString("\n**" + playlist + "**")
	}

	ctx.Reply(buf.String())
}

func cmdPlaylistShow(ctx *CommandContext, args []string) {
	if len(args) != 1 {
		ctx.SendUsage()
		return
	}

	playlist := args[0]
	tags, err := playlistTags(playlist)
	if handleCommandErrors(ctx, SqlError, err) {
		return
	}

	buf := bytes.Buffer{}
	buf.WriteString("`" + playlist + "`'s tags:\n")
	for _, tag := range tags {
		buf.WriteString("\n**" + tag + "**")
	}

	ctx.Reply(buf.String())
}

// Scheduler Commands

func cmdStop(ctx *CommandContext, args []string) {
	wasActive := Scheduler.Stop()
	if wasActive {
		ctx.Reply(OkMessage)
	} else {
		ctx.Reply(NoActiveScheduleMessage)
	}
}

func cmdNext(ctx *CommandContext, args []string) {
	wasActive := Scheduler.Next()
	if wasActive {
		ctx.Reply(OkMessage)
	} else {
		ctx.Reply(NoActiveScheduleMessage)
	}
}

// Backup Commands

func cmdExport(ctx *CommandContext, args []string) {
	taglist, err := allTags()
	if handleCommandErrors(ctx, SqlError, err) {
		return
	}

	buf := bytes.Buffer{}
	enc := csv.NewWriter(&buf)
	for _, tag := range taglist {
		enc.Write([]string{tag.Name, tag.AuthorID, tag.Url})
	}
	enc.Flush()

	ctx.Session.ChannelFileSendWithMessage(ctx.Event.ChannelID,
		"Your records, sire:", "bannerbard-export.csv", &buf)
	logger.Printf("Exported %d tags", len(taglist))
}

func cmdImport(ctx *CommandContext, args []string) {
	if len(ctx.Event.Attachments) != 1 {
		ctx.Reply("Sire, I need a single file attatched to that command.")
		return
	}

	resp, err := http.Get(ctx.Event.Attachments[0].URL)
	if handleCommandErrors(ctx, GeneralError, err) {
		return
	}
	defer resp.Body.Close()

	var maximumSize int64 = 1024 * 1024 // 1 MB
	if resp.ContentLength > maximumSize {
		ctx.Reply("Sire, that is most certainly not one of my exported files.")
		return
	}

	errs := []error{}

	err = clearTags()
	errs = append(errs, err)

	dec := csv.NewReader(resp.Body)
	lineno := 1
	for {
		record, err := dec.Read()
		if err == io.EOF {
			break
		}

		if handleCommandErrors(ctx, GeneralError, err) {
			return
		}

		if len(record) != 3 {
			errs = append(errs, errors.New(fmt.Sprintf(
				"I expected 3 entries on line %d, but I found %d.",
				lineno, len(record))))
			continue
		}

		tag, authorID, url := strings.TrimSpace(record[0]), record[1], record[2]
		err = insertTag(tag, authorID, url)
		errs = append(errs, err)
	}

	if !handleCommandErrors(ctx, GeneralError, errs...) {
		ctx.Reply("My memory is replaced with your new set, sire.")
	}
}
