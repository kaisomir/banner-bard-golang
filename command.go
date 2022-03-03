/*
 * Banner Bard: Banner-serving discord bot, sire.
 *
 * command.go - the command builder. Unlike discord.py, bwmarrin's
 * discordgo is intended to be a thin wrapper between Golang and the
 * Discord API -- this means that there's no bot builder, command
 * builder, or anything like that. So, I've implemented my own
 * here. banner-bard.go:init() should provide a good example how to
 * use the command builder, and is arguably clearer for making your
 * own commands than learning from this module itself.
 *
 *
 * This program uses the BSD 3-Clause license. You can find details under
 * the file LICENSE or under <https://opensource.org/licenses/BSD-3-Clause>.
 */
package main

import (
	"bytes"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"strings"
)

/* The permission bits. When giving a command a permission, you pick
 * and choose which groups get to run the command, binary-or them
 * together, and that's it.
 */
const (
	PermEveryone byte = 1 << iota
	PermManageServer
	PermRole
)

/*
 * When a command is called, it is provided with context of where the
 * command came from, which event was generated, the command struct
 * chosen, and so on. I also gave it a few convenience methods for
 * common procedures like CommandContext.Reply().
 */
type CommandContext struct {
	Session     *discordgo.Session
	Event       *discordgo.MessageCreate
	Command     Command // TODO should I make this a pointer instead? Copying the whole struct feels a bit wasteful.
	CommandName string
	Prefix      string
}

type CommandFunc func(ctx *CommandContext, args []string)

type Command interface {
	Apply(ctx *CommandContext, args []string)
	Help(ctx *CommandContext) string
	Perms() byte
	Usage() string
}

/*
 * A simple command applies its context and arguments to the command *
 * function given. You make a function, e.g. cmdHelp(), tack it to the
 * command builder, and viola! A simple command.
 */
type SimpleCommand struct {
	function    CommandFunc
	usage       string
	perms       byte
	description string
}

func (cmd *SimpleCommand) Apply(ctx *CommandContext, args []string) {
	cmd.function(ctx, args)
}

func (cmd *SimpleCommand) Help(ctx *CommandContext) string {
	switch {
	case !userPermitted(ctx, cmd):
		return ""
	case cmd.usage == "":
		return fmt.Sprintf("`%s`, %s\n", ctx.CommandName, cmd.description)
	default:
		return fmt.Sprintf("`%s %s`, %s\n", ctx.CommandName, cmd.usage, cmd.description)
	}
}

func (cmd *SimpleCommand) Perms() byte   { return cmd.perms }
func (cmd *SimpleCommand) Usage() string { return cmd.usage }

/*
 * A compound command combines multiple other commands in one
 * namespace. This feature is entirely used for the playlist commands so
 * that their names mirror the usual tag commands.
 */
type CompoundCommand struct {
	commandMap map[string]Command
	helpList   []string
	perms      byte
}

func (cmd *CompoundCommand) Apply(ctx *CommandContext, args []string) {
	subCmd, ok := cmd.commandMap[args[0]]
	if !ok {
		// TODO: some type of explicit error here that the
		// subcommand doesn't exist?
		return
	}

	if userPermitted(ctx, subCmd) {
		ctx.Command = subCmd
		ctx.CommandName += " " + args[0]
		logger.Printf("Invoked subcommand '%s'\n", ctx.CommandName)
		subCmd.Apply(ctx, args[1:])
	}
}

func (cmd *CompoundCommand) Help(ctx *CommandContext) string {
	buf := bytes.Buffer{}
	parentCommandName := ctx.CommandName
	defer func() { ctx.CommandName = parentCommandName }()

	for _, cmdName := range cmd.helpList {
		ctx.CommandName = parentCommandName + " " + cmdName
		buf.WriteString(cmd.commandMap[cmdName].Help(ctx))
	}

	return buf.String()
}

func (cmd *CompoundCommand) Perms() byte   { return cmd.perms }
func (cmd *CompoundCommand) Usage() string { return "CMD [ARGS...]" }

type CommandEvaluator struct {
	prelude    string
	commandMap map[string]Command
	helpText   []HelpNode
}

func (eval *CommandEvaluator) Help(ctx *CommandContext) string {
	buf := bytes.Buffer{}
	buf.WriteString(eval.prelude + "\n\n")

	for i, node := range eval.helpText {
		if node.isCommand {
			// It's a command name
			ctx.CommandName = ctx.Prefix + node.text
			buf.WriteString(eval.commandMap[node.text].Help(ctx))
		} else {
			// It's a heading
			if i > 0 {
				// Give an extra space if we're not at the top of the list
				buf.WriteRune('\n')
			}
			buf.WriteString("**" + node.text + "**\n")
		}
	}

	return buf.String()
}

type CompoundCommandBuilder struct{}

type HelpNode struct {
	isCommand bool   // true -> command, false -> heading
	text      string // command -> name, heading -> title
}

func userPermitted(ctx *CommandContext, cmd Command) bool {
	cmdPerms := cmd.Perms()
	if cmdPerms&PermEveryone == PermEveryone {
		// Everyone can run it.
		return true
	}

	if ctx.Event.Author.ID == Settings.OwnerID {
		// The owner can run it.
		return true
	}

	if cmdPerms&PermManageServer == PermManageServer {
		// Does the user have ManagerServer permissions?
		perms, err := ctx.Session.State.UserChannelPermissions(
			ctx.Event.Author.ID, ctx.Event.ChannelID)

		var requiredPerm int64 = discordgo.PermissionManageServer
		if err == nil && requiredPerm == perms&requiredPerm {
			return true
		}
	}

	if cmdPerms&PermRole == PermRole {
		// Does the user have one of the allowed roles?
		for _, allowedRole := range Settings.AllowedRoles {
			for _, authorRole := range ctx.Event.Member.Roles {
				if allowedRole == authorRole {
					return true
				}
			}
		}
	}

	// No conditions are met
	return false
}

// Context-sensitive helper functions

func (ctx *CommandContext) Reply(message string) {
	ctx.Session.ChannelMessageSend(ctx.Event.ChannelID, message)
}

func (ctx *CommandContext) SendUsage() {
	if ctx.Command.Usage() == "" {
		ctx.Reply(fmt.Sprintf("Usage: `%s`", ctx.CommandName))
	} else {
		ctx.Reply(fmt.Sprintf("Usage: `%s %s`", ctx.CommandName, ctx.Command.Usage()))
	}
}

// Command Evaluation

func evalCommand(s *discordgo.Session, m *discordgo.MessageCreate,
	evaluator *CommandEvaluator, prefix string) {

	content := m.Content[len(prefix):]
	args := strings.Split(content, " ")
	ctx := CommandContext{
		Session: s,
		Event:   m,
		Prefix:  prefix}

	cmd, ok := evaluator.commandMap[args[0]]
	if !ok {
		return
	}

	if userPermitted(&ctx, cmd) {
		ctx.Command = cmd
		ctx.CommandName = prefix + args[0]
		logger.Printf("Invoked command '%s' for user %s#%s %s\n",
			ctx.CommandName, m.Author.Username,
			m.Author.Discriminator, m.Author.Mention())

		cmd.Apply(&ctx, args[1:])
	}
}

// Command Evaluator Building

func BuildCommandEvaluator(prelude string) *CommandEvaluator {
	return &CommandEvaluator{
		prelude:    prelude,
		commandMap: make(map[string]Command),
		helpText:   []HelpNode{}}
}

func (builder *CommandEvaluator) Simple(name string, function CommandFunc,
	desc string, usage string, perms byte) *CommandEvaluator {

	builder.helpText = append(builder.helpText, HelpNode{
		isCommand: true,
		text:      name})

	builder.commandMap[name] = &SimpleCommand{
		function:    function,
		description: desc,
		usage:       usage,
		perms:       perms}

	return builder
}

func (builder *CommandEvaluator) Compound(
	name string, cmd *CompoundCommand) *CommandEvaluator {

	builder.helpText = append(builder.helpText, HelpNode{
		isCommand: true,
		text:      name})

	builder.commandMap[name] = cmd

	return builder
}

func (builder *CommandEvaluator) Group(title string) *CommandEvaluator {
	builder.helpText = append(builder.helpText, HelpNode{
		isCommand: false,
		text:      title})

	return builder
}

func (builder *CommandEvaluator) Done() CommandEvaluator {
	return *builder
}

// Compound command building

func BuildCompoundCommand(perms byte) *CompoundCommand {
	return &CompoundCommand{
		commandMap: make(map[string]Command),
		perms:      perms}
}

func (builder *CompoundCommand) Simple(name string, function CommandFunc,
	desc string, usage string, perms byte) *CompoundCommand {

	builder.helpList = append(builder.helpList, name)

	builder.commandMap[name] = &SimpleCommand{
		function:    function,
		description: desc,
		usage:       usage,
		perms:       perms}

	return builder
}
