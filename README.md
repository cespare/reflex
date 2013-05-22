# Reflex

Reflex is a small tool to watch a directory and rerun a command when certain files change.

## Installation

    $ go get github.com/cespare/reflex

Note that this has one dependency outside of the go standard library: `github.com/howeyc/fsnotify`. `go get`
will automatically fetch it for you.

## Usage

    $ reflex [OPTIONS] [COMMAND]

`OPTIONS` are:

* `-h`: Display the help
* `-r REGEX`: A [Go regular expression](http://golang.org/pkg/regexp/) to match paths.

`COMMAND` is any command you'd like to run. Any instance of `{}` will be replaced with the filename of changed
file.

## Tips

* If you don't use `-r`, reflex will match every file. (It prints a warning, but if this is what you intend
  you can safely ignore it.)
* Many regex characters are interpreted specially by various shells. You'll generally want to minimize this
  effect by putting the regex in single quotes.
* If your command has options, you'll probably need to use `--` to separate the reflex flags from your command
  flags. For example: `reflex -r '.*\.txt' -- ls -l`.
* If you're going to use shell things, you might need to invoke a shell as a parent process:
  `reflex -- bash -c 'sleep 1 && echo {}'`

## Examples

    # Print every file when it changes
    reflex echo {}
    # Run make when any .c file changes
    reflex -r '\.c$' make
    # TODO: more examples

## Notes

TODO: Describe the two different batching strategies.

## TODO

* Check that I'm handling command failure well.
* Handle the inverse (restart) case, for servers.
* Accept a config file (like Guardfile, but much simpler). Just a list of regexes + commands.
* Options: specify only files or only directories.
* Options: Allow force non-recursive / exclude a dir?
* Options: Change the substitution symbol from {} to something else.
* Allow for shell globbing (+recursive?) as well as regex matching.
