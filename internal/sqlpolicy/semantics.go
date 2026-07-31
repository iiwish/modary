package sqlpolicy

import "strings"

// HasTemporarySchemaReference reports whether tokens contain a quoted or
// unquoted temp schema qualifier.
func HasTemporarySchemaReference(tokens []Token) bool {
	for index := 0; index+1 < len(tokens); index++ {
		identifier := tokens[index].Word
		if tokens[index].Identifier != "" {
			identifier = tokens[index].Identifier
		} else if tokens[index].String != "" {
			identifier = tokens[index].String
		}
		if strings.EqualFold(identifier, "temp") && tokens[index+1].Symbol == "." {
			return true
		}
	}
	return false
}
