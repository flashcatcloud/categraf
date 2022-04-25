package parser

import "github.com/toolkits/pkg/container/list"

type Parser interface {
	Parse(input []byte, slist *list.SafeList) error
}
