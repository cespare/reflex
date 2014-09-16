package main

import "regexp"

var defaultExcludes = []string{
	// VCS dirs
	`(^|/)\.git/`,
	`(^|/)\.hg/`,
	// Vim
	`~$`,
	`\.swp$`,
	// Emacs
	`\.#`,
	`(^|/)#.*#$`,
	// OS X
	`(^|/)\.DS_Store$`,
}

var defaultExcludeMatcher multiMatcher

func init() {
	for _, pattern := range defaultExcludes {
		m := &regexMatcher{regex: regexp.MustCompile(pattern), inverse: true}
		defaultExcludeMatcher = append(defaultExcludeMatcher, m)
	}
}
