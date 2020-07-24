package main

import "strings"

// Split splits the passed text into chunks and concatenates them back to
// a list of strings where the length of each string in that list does
// not exceed maxNumChars
func Split(text, delimiter string, maxNumChars int) []string {

	// no split needed
	if len(text) <= maxNumChars {
		return []string{text}
	}

	// max number of strings needed
	result := make([]string, 0, (len(text)/maxNumChars)+1)

	// reserve enough space
	var sb strings.Builder
	sb.Grow(int(float64(1.5) * float64(maxNumChars)))

	// split text into e.g. lines
	tokens := strings.Split(text, delimiter)

	expectedGrowth := 0
	tokenSuffix := ""
	for idx, token := range tokens {

		if idx < len(tokens)-1 {
			// append delimiter
			expectedGrowth = len(token) + len(delimiter)
			tokenSuffix = delimiter

		} else {
			// no delimiter appended
			expectedGrowth = len(token)
			tokenSuffix = ""
		}

		// string can fit token and delimiter
		if expectedGrowth > maxNumChars {
			// edge case where the text between delimiters surpasses
			// size requirements, force write
			sb.WriteString(token)
			sb.WriteString(tokenSuffix)
		} else if sb.Len()+expectedGrowth <= maxNumChars {
			sb.WriteString(token)
			sb.WriteString(tokenSuffix)
		} else {
			// string length would exceed maxNumChars
			result = append(result, sb.String())
			sb.Reset()

			// write the next token & delimiter pair
			sb.WriteString(token)
			sb.WriteString(tokenSuffix)
		}

	}

	if sb.Len() > 0 {
		result = append(result, sb.String())
	}

	return result
}
