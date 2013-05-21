# Reflex

Reflex is a small tool to watch a directory and rerun a command when certain files change.

## Installation

## Usage

## TODO

* Basic working version: match a regex and run a command.
* Handle command failure well.
* Batch frequent changes together to avoid running the command too much.
* Take care of the case where the files change again while the command is running.
* Handle the inverse (restart) case, for servers.
* Accept a config file (like Guardfile, but much simpler). Just a list of regexes + commands.
* Options: specify only files or only directories.
* Allow for some kind of substitution syntax for the filename.
