# Reflex

Reflex is a small tool to watch a directory and rerun a command when certain files change.

## Installation

    $ go get github.com/cespare/reflex

Note that this has some dependencies outside of the Go standard library. You'll need git installed, and `go
get` will automatically fetch them for you.

## Usage

    $ reflex [OPTIONS] [COMMAND]

`OPTIONS` are:

* `-h`: Display the help
* `-r REGEX`: A [Go regular expression](http://golang.org/pkg/regexp/) to match paths.

TODO: fill out all the options after this has settled down a bit.

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

* Handle the inverse (restart) case, for servers.
* Implement recursive globbing.
* Document file argument list splitting behavior (with go-shellquote)
* Clean up the readme when the interface has settled down.

* Implement/copy the parts of go-shellquote that I need myself.
* Add more debugging info to -v.
* Consider vendoring all the deps.
* Fix/remove TODOs.
