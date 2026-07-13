package graph

import (
	"net/http"
	"strings"

	"tangled.org/cuducos.me/go-cnpj"
)

type requestType int

const (
	singleID requestType = iota
	connection
	badRequest
)

type graphRequest struct {
	kind     requestType
	id1, id2 string
}

func isValidID(p string) bool {
	if cnpj.IsValid(p) {
		return true
	}
	if len(p) != 32 {
		return false
	}
	for i := range 32 {
		if (p[i] < 'a' || p[i] > 'z') && (p[i] < '0' || p[i] > '9') {
			return false
		}
	}
	return true
}

func ids(p string) (string, string, bool) {
	if len(p) > 14 && p[14] == '/' {
		one := p[:14]
		two := p[15:]
		if isValidID(one) && isValidID(two) {
			return one, two, true
		}
	}
	if len(p) > 18 && p[18] == '/' {
		one := p[:18]
		two := p[19:]
		if isValidID(one) && isValidID(two) {
			return one, two, true
		}
	}
	if len(p) > 32 && p[32] == '/' {
		one := p[:32]
		two := p[33:]
		if isValidID(one) && isValidID(two) {
			return one, two, true
		}
	}
	return "", "", false
}

func parseRequest(r *http.Request) *graphRequest {
	p := strings.TrimPrefix(r.URL.Path, "/")
	p = strings.TrimSuffix(p, "/")

	if isValidID(p) {
		if cnpj.IsValid(p) {
			p = cnpj.Unmask(p)
		}
		return &graphRequest{singleID, p, ""}
	}

	one, two, ok := ids(p)
	if ok {
		if cnpj.IsValid(one) {
			one = cnpj.Unmask(one)
		}
		if cnpj.IsValid(two) {
			two = cnpj.Unmask(two)
		}
		return &graphRequest{connection, one, two}
	}

	return &graphRequest{kind: badRequest}
}
