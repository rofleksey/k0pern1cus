package util

type ContextKey string

func (c ContextKey) String() string {
	return "k0pern1cus_" + string(c)
}

var UsernameContextKey ContextKey = "username"
var IpContextKey ContextKey = "ip"
