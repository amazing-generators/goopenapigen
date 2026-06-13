package goident

import (
	"go/token"
)

// // // // // // // // // //

// IsPackageName reports whether the string is a valid Go package name.
func IsPackageName(packageName string) bool {
	if packageName == "" || token.Lookup(packageName).IsKeyword() {
		return false
	}

	for index, symbol := range packageName {
		switch {
		case symbol == '_':
		case symbol >= 'a' && symbol <= 'z':
		case symbol >= 'A' && symbol <= 'Z':
		case index > 0 && symbol >= '0' && symbol <= '9':
		default:
			return false
		}
	}

	return true
}
