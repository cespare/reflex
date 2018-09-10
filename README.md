# Reflex

Reflex is a small tool to watch a directory and rerun a command when certain
files change. It's great for automatically running compile/lint/test tasks and
for reloading your application when the code changes.

## A simple example

    # Rerun make whenever a .c file changes
    reflex -r '\.c$' make

## Installation

You'll need Go 1.11+ installed:

    $ go get github.com/cespare/reflex

Reflex probably only works on Linux and Mac OS.

TODO: provide compiled downloads for linux/darwin amd64.

## Usage

The following is given by running `reflex -h`:

```
Usage: reflex [OPTIONS] [COMMAND]

COMMAND is any command you'd like to run. Any instance of {} will be replaced
with the filename of the changed file. (The symbol may be changed with the
--substitute flag.)

OPTIONS are given below:
      --all=false:
            Include normally ignored files (VCS and editor special files).
  -c, --config="":
            A configuration file that describes how to run reflex
            (or '-' to read the configuration from stdin).
  -d, --decoration="plain":
            How to decorate command output. Choices: none, plain, fancy.
  -g, --glob=[]:
            A shell glob expression to match filenames. (May be repeated.)
  -G, --inverse-glob=[]:
            A shell glob expression to exclude matching filenames.
            (May be repeated.)
  -R, --inverse-regex=[]:
            A regular expression to exclude matching filenames.
            (May be repeated.)
      --only-dirs=false:
            Only match directories (not files).
      --only-files=false:
            Only match files (not directories).
  -r, --regex=[]:
            A regular expression to match filenames. (May be repeated.)
  -e, --sequential=false:
            Don't run multiple commands at the same time.
  -t, --shutdown-timeout=500ms:
            Allow services this long to shut down.
  -s, --start-service=false:
            Indicates that the command is a long-running process to be
            restarted on matching changes.
      --substitute="{}":
            The substitution symbol that is replaced with the filename
            in a command.
  -v, --verbose=false:
            Verbose mode: print out more information about what reflex is doing.

Examples:

    # Print each .txt file if it changes
    $ reflex -r '\.txt$' echo {}

    # Run 'make' if any of the .c files in this directory change:
    $ reflex -g '*.c' make

    # Build and run a server; rebuild and restart when .java files change:
    $ reflex -r '\.java$' -s -- sh -c 'make && java bin/Server'
```

### Overview

Reflex watches file changes in the current working directory and re-runs the
command that you specify. The flags change what changes cause the command to be
rerun and other behavior.

### Patterns

You can specify files to match using either shell glob patterns (`-g`) or
regular expressions (`-r`). If you don't specify either, reflex will run your
command after any file changes. (Reflex ignores some common editor and version
control files; see Ignored files, below.)

You can specify inverse matches by using the `--inverse-glob` (`-G`) and
`--inverse-regex` (`-R`) flags.

If you specify multiple globs/regexes (e.g. `-r foo -r bar -R baz -G x/*/y`),
only files that match all patterns and none of the inverse patterns are
selected.

The shell glob syntax is described
[here](http://golang.org/pkg/path/filepath/#Match), while the regular expression
syntax is described [here](https://code.google.com/p/re2/wiki/Syntax).

The path that is matched against the glob or regular expression does not have a
leading `./`. For example, if there is a file `./foobar.txt` that changes, then
it will be matched by the regular expression `^foobar`. If the path is a
directory, it has a trailing `/`.

### --start-service

The `--start-service` flag (short version: `-s`) inverts the behavior of command
running: it runs the command when reflex starts and kills/restarts it each time
files change. This is expected to be used with an indefinitely-running command,
such as a server. You can use this flag to relaunch the server when the code is
changed.

### Substitution

Reflex provides a way for you to determine, inside your command, what file
changed. This is via a substitution symbol. The default is `{}`. Every instance
of the substitution symbol inside your command is replaced by the filename.

As a simple example, suppose you're writing Coffeescript and you wish to compile
the CS files to Javascript when they change. You can do this with:

    $ reflex -r '\.coffee$' -- coffee -c {}

In case you need to use `{}` for something else in your command, you can change
the substitution symbol with the `--substitute` flag.

### Configuration file

What if you want to run many watches at once? For example, when writing web
applications I often want to rebuild/rerun the server when my code changes, but
also build SCSS and Coffeescript when those change as well. Instead of running
multiple reflex instances, which is cumbersome (and inefficient), you can give
reflex a configuration file.

The configuration file syntax is simple: each line is a command, and each
command is composed of flags and arguments -- just like calling reflex but
without the initial `reflex`. Lines that start with `#` are ignored. Commands
can span multiple lines if they're \\-continued, or include multi-line strings.
Here's an example:

    # Rebuild SCSS when it changes
    -r '\.scss$' -- \
       sh -c 'sass {} `basename {} .scss`.css'
    
    # Restart server when ruby code changes
    -sr '\.rb$' -- \
        ./bin/run_server.sh

If you want to change the configuration file and have reflex reload it on the
fly, you can run reflex inside reflex:

    reflex -s -g reflex.conf -- reflex -c reflex.conf

This tells reflex to run another reflex process as a service that's restarted
whenever `reflex.conf` changes.

### --sequential

When using a config file to run multiple simultaneous commands, reflex will run
them at the same time (if appropriate). That is, a particular command can only
be run once a previous run of that command finishes, but two different commands
may run at the same time. This is usually what you want (for speed).

As a concrete example, consider this config file:

    -- sh -c 'for i in `seq 1 5`; do sleep 0.1; echo first; done'
    -- sh -c 'for i in `seq 1 5`; do sleep 0.1; echo second; done'

When this runs, you'll see something like this:

    [01] second
    [00] first
    [01] second
    [00] first
    [00] first
    [01] second
    [01] second
    [00] first
    [01] second
    [00] first

Note that the output is interleaved. (Reflex does ensure that each line of
output is not interleaved with a different line.) If, for some reason, you need
to ensure that your commands don't run at the same time, you can do this with
the `--sequential` (`-e`) flag. Then the output would look like (for example):

    [01] second
    [01] second
    [01] second
    [01] second
    [01] second
    [00] first
    [00] first
    [00] first
    [00] first
    [00] first

### Decoration

By default, each line of output from your command is prefixed with something
like `[00]`, which is simply an id that reflex assigns to each command. You can
use `--decoration` (`-d`) to change this output: `--decoration=none` will print
the output as is; `--decoration=fancy` will color each line differently
depending on which command it is, making it easier to distinguish the output.

### Ignored files

Reflex ignores a variety of version control and editor metadata files by
default. If you wish for these to be included, you can provide reflex with the
`--all` flag.

You can see a list of regular expressions that match the files that reflex
ignores by default
[here](https://github.com/cespare/reflex/blob/master/defaultexclude.go#L5).

## Notes and Tips

If you don't use `-r` or `-g`, reflex will match every file.

Reflex only considers file creation and modification changes. It does not report
attribute changes nor deletions.

For ignoring directories, it's easiest to use a regular expression: `-R '^dir/'`.

Many regex and glob characters are interpreted specially by various shells.
You'll generally want to minimize this effect by putting the regex and glob
patterns in single quotes.

If your command has options, you'll probably need to use `--` to separate the
reflex flags from your command flags. For example: `reflex -r '.*\.txt' -- ls
-l`.

If you're going to use shell things, you need to invoke a shell as a parent
process:

    reflex -- sh -c 'sleep 1 && echo {}'

If your command is running with sudo, you'll need a passwordless sudo, because
you cannot enter your password in through reflex.

It's not difficult to accidentally make an infinite loop with certain commands.
For example, consider this command: `reflex -r '\.txt' cp {} {}.bak`. If
`foo.txt` changes, then this will create `foo.txt.bak`, `foo.txt.bak.bak`, and
so forth, because the regex `\.txt` matches each file. Reflex doesn't have any
kind of infinite loop detection, so be careful with commands like `cp`.

The restart behavior works as follows: if your program is still running, reflex
sends it SIGINT; after 1 second if it's still alive, it gets SIGKILL. The new
process won't be started up until the old process is dead.

### Batching

Part of what reflex does is apply some heuristics to batch together file
changes. There are many reasons that files change on disk, and these changes
frequently come in large bursts. For instance, when you save a file in your
editor, it probably makes a tempfile and then copies it over the target, leading
to several different changes. Reflex hides this from you by batching some
changes together.

One thing to note, though, is that the the batching is a little different
depending on whether or not you have a substitution symbol in your command. If
you do not, then updates for different files that all match your pattern can be
batched together in a single update that only causes your command to be run
once.

If you are using a substitution symbol, however, each unique matching file will
be batched separately.

### Argument list splitting

When you give reflex a command from the commandline (i.e., not in a config
file), that command is split into pieces by whatever shell you happen to be
using. When reflex parses the config file, however, it must do that splitting
itself. For this purpose, it uses [this library](https://github.com/kballard/go-shellquote)
which attempts to match `sh`'s argument splitting rules.

This difference can lead to slightly different behavior when running commands
from a config file. If you're confused, it can help to use `--verbose` (`-v`)
which will print out each command as interpreted by reflex.

### Open file limits

Reflex currently must hold an open file descriptor for every directory it's
watching, recursively. If you run reflex at the top of a big directory tree, you
can easily run into file descriptor limits. You might see an error like this:

    open some/path: too many open files

There are several things you can do to get around this problem.

1. Run reflex in the most specific directory possible. Don't run
   `reflex -g path/to/project/*.c ...` from `$HOME`; instead run reflex in
   `path/to/project`.
2. Ignore large subdirectories. Reflex already ignores, for instance, `.git/`.
   If you have other large subdirectories, you can ignore those yourself:
   `reflex -R '^third_party/' ...` ignores everything under `third_party/` in
   your project directory.
3. Raise the fd limit using `ulimit` or some other tool. On some systems, this
   might default to a restrictively small value like 256.

See [issue #6](https://github.com/cespare/reflex/issues/6) for some more
background on this issue.

## The competition

* https://github.com/guard/guard
* https://github.com/alexch/rerun
* https://github.com/mynyml/watchr
* https://github.com/eaburns/Watch
* https://github.com/alloy/kicker

### Why you should use reflex instead

* Reflex has no dependencies. No need to install Ruby or anything like that.
* Reflex uses an appropriate file watching mechanism to watch for changes
  efficiently on your platform.
* Reflex gives your command the name of the file that changed.
* No DSL to learn -- just give it a shell command.
* No plugins.
* Not tied to any language, framework, workflow, or editor.

## Authors

* Benedikt Böhm ([hollow](https://github.com/hollow))
* Caleb Spare ([cespare](https://github.com/cespare))
* PJ Eby ([pjeby](https://github.com/pjeby))
* Rich Liebling ([rliebling](https://github.com/rliebling))
* Seth W. Klein ([sethwklein](https://github.com/sethwklein))
* Vincent Vanackere ([vanackere](https://github.com/vanackere))
