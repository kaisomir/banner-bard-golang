HACKING.md or: How I Learned to Stop Worring and Love Maintining For Free

## FAQ: Fully Anticipated Questions

Before a proper introduction, a few foreseen questions:

- Q: For `go get .`, **I'm getting `go get: no install location for
  directory .../.../...`**
- A: That's completely fine and okay, Go is just a bit whiney if you
  don't install it in $GOPATH/src/banner-bard or the like. Still
  works perfectly fine as-is, though.

- Q: **Why did you choose Golang for a bot and not discord.py?**
- A: I liked writing in Go over Python, and having to set up pyvenv to
  make an entire virtual environment because it's "good practice" is
  a really yellow flag for me for how the Python ecosystem is.

- Q: **How does do Linux?**
- A: I assume Linux for the bot, because I use Linux daily and setting up a
  Linux VPS means I won't have do pray my Windows laptop won't suspend or shut
  down, killing the bot in the process. If you don't ~~want to~~ spend hours
  learning Linux just to set up a stupid bot that changes banner images, then
  nicely asking dino, lyric, mip5, or cube might have them set a VM up for you.
- Also, I've prepared a sustemd Unit file that manages the program as a proper
  daemon. Move it to `/etc/systemd/system/` and run as root `systemctl enable
  bard`. It assumes you have the program in its own directory at `/srv/bard/`,
  prepared to run as a dedicated user, `bard`.  You should modify it otherwise.

## Prerequisites

Maintaining this discord bot implies you have some surface-level
knowledge of Golang and SQLite. If you don't know Golang, the tour is
found at <https://tour.golang.org/>, and if you're not familiar with
SQLite, there's a tutorialspoint section at
<https://www.tutorialspoint.com/sqlite/sqlite_commands.htm> and the
way to do it with Golang is
<https://golang.org/pkg/database/sql/>.

Don't feel like you have to master all of these before you work on
this bot -- if you know any coding at all, you might probably learn
faster (though less thorough and more likely to pick up my bad habits)
by taking a blind jump and reading the source code.

## Bot Structure

The bot (as of this documentation) is split into four distinct
modules:

- `db.go`, which handles talking to the SQLite database,
- `command.go`, which is the library that builds and evaluates
  commands,
- `scheduler.go`, which schedules banner tags, and
- `banner-bard.go`, which houses the heart of the banner bard.

In the (anticipated) likelist order you want to maintain the bot:

- **I want to add a command to the bard** - `banner-bard.go`
- **The way the bard is handling commands are broken** - `commands.go`
- **I want to make the bard remember more stuff** - `db.go`
- **I want to add another way for the bard to play through tags** -
  `scheduler.go`

Each individual file has more in-depth documentation about itself.

## Final Notes

While the bot is finished for me, ther emight be some latent bugs that I've yet
to fix. Because it's difficult to determine what parts of code are meaningless
quirks vs. load-bearing quirks, I've done a somewhat passable job to comment
the "Why" for some of the code, because by the time there's a bug it's probably
too late to contact me.

I have shut off my personal computer for good. My mobile phone is disconnected
and melted with thermite. I've burnt all my identification, money, and papers.
I have commenced my walk eastward.. into the rising sun.
