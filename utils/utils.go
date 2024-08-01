package utils

import (
	"fmt"
	"strings"
	"unicode"
)

// convert ObjectName -> []string{"object","name"} and error on unexpected chars
// updated to preserve UserID -> []string{"user", "id"}
func SplitObjWords(oName string) (ret []string, err error) {

	if len(oName) == 0 {
		return nil, fmt.Errorf("zero length input")
	}

	lastWordBreak := 0
	lastUpperIdx := 0
	upperCount := 0
	for idx := 1; idx < len(oName); idx++ {

		c := rune(oName[idx])

		//log.Printf("rune: %v", string(oName[idx]))

		if unicode.IsDigit(c) {
			continue
		}

		if unicode.IsLetter(c) {

			if unicode.IsLower(c) {
				if lastUpperIdx == idx-1 && upperCount > 1 {
					// word break
					ret = append(ret, strings.ToLower(oName[lastWordBreak:idx-1]))
					lastWordBreak = idx - 1
				}
				upperCount = 0
				continue
			}

			if lastUpperIdx == idx-1 {
				lastUpperIdx = idx
				upperCount++
				continue
			}

			// word break
			ret = append(ret, strings.ToLower(oName[lastWordBreak:idx]))
			lastWordBreak = idx

			if unicode.IsUpper(c) {
				lastUpperIdx = idx
				upperCount++
			}

			continue
		}

		// some other character
		return nil, fmt.Errorf("invalid character %q", c)
	}

	// append last
	ret = append(ret, strings.ToLower(oName[lastWordBreak:]))

	return ret, nil
}
