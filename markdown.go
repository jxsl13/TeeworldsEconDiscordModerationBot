package main

import (
	"fmt"
	"strings"
)

var (
	// is thread safe/goroutine safe
	markdownReplacer = strings.NewReplacer(
		"\\", "\\\\",
		"`", "\\`",
		"*", "\\*",
		"_", "\\_",
		"{", "\\{",
		"}", "\\}",
		"[", "\\[",
		"]", "\\]",
		"(", "\\(",
		")", "\\)",
		"#", "\\#",
		"+", "\\+",
		"-", "\\-",
		".", "\\.",
		"!", "\\!",
	)
)

// Escape user input outside of inline code blocks
func Escape(userInput string) string {
	return markdownReplacer.Replace(userInput)
}

// WrapInInlineCodeBlock puts the user input into a inline codeblock that is properly escaped.
func WrapInInlineCodeBlock(userInput string) (userOutput string) {
	if len(userInput) == 0 {
		return ""
	}
	numberBackticks := strings.Count(userInput, "`") + 1

	userOutput = userInput
	for idx := 0; idx < numberBackticks; idx++ {
		userOutput = fmt.Sprintf("`%s`", userOutput)
	}
	return
}
