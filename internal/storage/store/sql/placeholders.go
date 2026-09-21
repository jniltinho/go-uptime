// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package sql

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"regexp"
	"sync"
)

// maximumCachedTranslations bounds the cache of translated queries: the queries of the store are constants, except for
// the IN lists built dynamically, which only vary with the number of items
const maximumCachedTranslations = 1024

var (
	errInvalidPlaceholder   = errors.New("invalid placeholder")
	errArgumentCountInvalid = errors.New("wrong number of arguments")
	errNamedArgument        = errors.New("named arguments are not supported")
)

// translatedQuery is a query whose PostgreSQL placeholders ($1, $2, ...) were replaced by MySQL placeholders (?)
type translatedQuery struct {
	// query is the query with ? placeholders
	query string

	// argumentIndexes has, for each ? of query, the zero-based index of the argument it refers to
	argumentIndexes []int

	// numberOfArguments is the highest placeholder number referenced by the query, which is the number of arguments it
	// requires, as with PostgreSQL
	numberOfArguments int

	// insertWithoutReturning and returningColumn are set when the query is an INSERT ... RETURNING <column>, which MySQL
	// does not support: see mysqlConn.queryInsertReturning
	insertWithoutReturning string
	returningColumn        string
}

// insertReturningPattern matches an INSERT whose last clause is RETURNING with a single column
var insertReturningPattern = regexp.MustCompile(`(?is)^\s*(INSERT\s.*?)\s+RETURNING\s+([A-Za-z_][A-Za-z0-9_]*)\s*$`)

var (
	translationsMutex sync.Mutex
	translations      = make(map[string]*translatedQuery)
)

// translatePlaceholders returns the query with its $N placeholders replaced by ?, in order of appearance. Placeholders
// inside string literals, quoted identifiers and comments are left untouched. It fails on $0, on a $ followed by a
// number too large, and on a ? outside of literals and comments, which would be taken as a placeholder by MySQL.
func translatePlaceholders(query string) (*translatedQuery, error) {
	translationsMutex.Lock()
	cached, exists := translations[query]
	translationsMutex.Unlock()
	if exists {
		return cached, nil
	}
	translated, err := parsePlaceholders(query)
	if err != nil {
		return nil, err
	}
	translationsMutex.Lock()
	if len(translations) >= maximumCachedTranslations {
		translations = make(map[string]*translatedQuery)
	}
	translations[query] = translated
	translationsMutex.Unlock()
	return translated, nil
}

func parsePlaceholders(query string) (*translatedQuery, error) {
	translated := &translatedQuery{}
	output := make([]byte, 0, len(query))
	for i := 0; i < len(query); {
		character := query[i]
		switch {
		case character == '\'' || character == '"' || character == '`':
			end := endOfQuoted(query, i)
			output = append(output, query[i:end]...)
			i = end
		case character == '#' || (character == '-' && isLineCommentStart(query, i)):
			end := endOfLine(query, i)
			output = append(output, query[i:end]...)
			i = end
		case character == '/' && i+1 < len(query) && query[i+1] == '*':
			end := endOfBlockComment(query, i)
			output = append(output, query[i:end]...)
			i = end
		case character == '?':
			return nil, fmt.Errorf("%w: unexpected ? at position %d", errInvalidPlaceholder, i)
		case character == '$' && i+1 < len(query) && isDigit(query[i+1]):
			number, end := 0, i+1
			for ; end < len(query) && isDigit(query[end]); end++ {
				number = number*10 + int(query[end]-'0')
				if number > 65535 {
					return nil, fmt.Errorf("%w: placeholder number too large at position %d", errInvalidPlaceholder, i)
				}
			}
			if number == 0 {
				return nil, fmt.Errorf("%w: $0 at position %d", errInvalidPlaceholder, i)
			}
			output = append(output, '?')
			translated.argumentIndexes = append(translated.argumentIndexes, number-1)
			translated.numberOfArguments = max(translated.numberOfArguments, number)
			i = end
		default:
			output = append(output, character)
			i++
		}
	}
	translated.query = string(output)
	if matches := insertReturningPattern.FindStringSubmatch(translated.query); matches != nil {
		translated.insertWithoutReturning, translated.returningColumn = matches[1], matches[2]
	}
	return translated, nil
}

// arguments returns the arguments of the translated query, repeated and reordered for its ? placeholders. Like
// PostgreSQL, it requires exactly as many arguments as the highest placeholder number.
func (translated *translatedQuery) arguments(arguments []driver.NamedValue) ([]driver.NamedValue, error) {
	if len(arguments) != translated.numberOfArguments {
		return nil, fmt.Errorf("%w: got %d arguments but the query requires %d", errArgumentCountInvalid, len(arguments), translated.numberOfArguments)
	}
	for _, argument := range arguments {
		if len(argument.Name) > 0 {
			return nil, errNamedArgument
		}
	}
	reordered := make([]driver.NamedValue, len(translated.argumentIndexes))
	for position, index := range translated.argumentIndexes {
		reordered[position] = driver.NamedValue{Ordinal: position + 1, Value: arguments[index].Value}
	}
	return reordered, nil
}

// endOfQuoted returns the index after the string literal or quoted identifier starting at start. A doubled quote
// escapes the quote, and in string literals a backslash escapes the next character, as in MySQL without
// NO_BACKSLASH_ESCAPES. An unterminated literal extends to the end of the query.
func endOfQuoted(query string, start int) int {
	quote := query[start]
	for i := start + 1; i < len(query); i++ {
		switch {
		case query[i] == '\\' && quote == '\'':
			i++
		case query[i] == quote:
			if i+1 < len(query) && query[i+1] == quote {
				i++
				continue
			}
			return i + 1
		}
	}
	return len(query)
}

// isLineCommentStart returns whether the -- at start begins a comment: MySQL requires a whitespace or control
// character after it
func isLineCommentStart(query string, start int) bool {
	if start+1 >= len(query) || query[start+1] != '-' {
		return false
	}
	return start+2 == len(query) || query[start+2] <= ' '
}

func endOfLine(query string, start int) int {
	for i := start; i < len(query); i++ {
		if query[i] == '\n' {
			return i + 1
		}
	}
	return len(query)
}

func endOfBlockComment(query string, start int) int {
	for i := start + 2; i+1 < len(query); i++ {
		if query[i] == '*' && query[i+1] == '/' {
			return i + 2
		}
	}
	return len(query)
}

func isDigit(character byte) bool {
	return character >= '0' && character <= '9'
}
