# THIS IS NO LONGER BEING MAINTAINED
With Discord's introduction of slash commands, discord.go's implementation has discouraged me from updating, and as such, I will be recreating this in Python. The repository should soon be viewable on my GitHub, if it is not already.
- kaisomir

# Banner Bard
Banner-serving discord bot, sire.

Forked from lawful-lazy's banner-bard.

## Build and Deploy

This bot depends on golang and sqlite3 to function. After installing
both and downloaing `banner-bard`, open a shell:

    $ cd banner-bard

Skip these unless you get the `go: go.mod file not found in current directory or any parent directory; see 'go help modules'` error.

    $ go mod init banner-bard
    $ go mod tidy

Copy example to config file:

    $ cp settings.json.example settings.json
    $ nano settings.json

Fill in fields in settings.json with your bot's settings, then continue.

    $ go build
    $ ./banner-bard

For long-term deployment on a server, see [the hacking guide](./HACKING.md).

## Commands

- `bb, help`, to show a synopsis of all my commands
- Tags
  - `bb, new TAG URL`, to make a new tag or replace a preexisting tag
  - `bb, del TAG`, to delete a preexisting tag
  - `bb, set TAG`, to set the banner to a tag
  - `bb, shuffle INTERVAL TAGS...`, to shuffle through multiple tags over time
  - `bb, cycle INTERVAL TAGS...`, to cycle through ordered tags over time
  - `bb, play INTERVAL TAGS...`, to play through tags once only over time
  - `bb, ls`, to list all tags
  - `bb, show TAG`, to show the tag's description
- Playlists
  - `bb, playlist new PLAYLIST TAGS...`, to create or replace a new playlist
  - `bb, playlist add PLAYLIST TAGS...`, to add tags to a playlist
  - `bb, playlist rm PLAYLIST TAGS...`, to remove tags from a playlist
  - `bb, playlist del PLAYLIST`, to delete a playlist
  - `bb, playlist shuffle INTERVAL PLAYLIST`, to shuffle through a playlist over time
  - `bb, playlist cycle INTERVAL PLAYLIST`, to cycle through the playlist over time
  - `bb, playlist play INTERVAL PLAYLIST`, to go through a playlist once only over time
  - `bb, playlist ls`, to list all playlists
  - `bb, playlist show PLAYLIST`, to show the tags in a playlist
- Scheduler
  - `bb, stop`, to stop playing through the banner queue
  - `bb, next`, to skip to the next tag in the banner queue
- Backups
  - `bb, export`, to upload all tags as a csv file.
  - `bb, import`, to import tags from a csv file.
