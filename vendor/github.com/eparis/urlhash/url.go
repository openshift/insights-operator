package urlhash

import (
	"crypto/sha256"
	"fmt"
	"math/rand"
	"net"
	"net/url"
	"strings"
)

var (
	globalSalt     = ""
	allowedWords   = map[string]struct{}{}
	OpenShiftWords = map[string]struct{}{
		"kubernetes": {},
		"k8s":        {},
		"openshift":  {},
		"console":    {},
		"api":        {},
		"com":        {},
		"net":        {},
		"org":        {},
	}
)

// Returns the sha256 value of salt+value
func hash(value string, salt string) string {
	input := salt + value
	output := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", output)
}

// Returns the last len(value) sha256 values of salt+value
func hashTrunc(value string, salt string, size int) string {
	hashVal := hash(value, salt)
	hashLen := len(hashVal)
	if hashLen > size {
		hashVal = hashVal[hashLen-size:]
	}
	return hashVal
}

// Returns the len(word) hash of a 'word'.
// returns the word itself if it is 'allowed'
func hashWord(word, salt string) string {
	if _, ok := allowedWords[word]; ok {
		return word
	}
	return hashTrunc(word, salt, len(word))
}

// Walk the text of an address, split by `sep` and return the hash (of len trunc)
// so 1.2.3.4 may return abc.def.g12.345
// and 2001:db8::1 may return dead:beef::1234
func hashIPHelper(ipStr string, salt string, sep string, trunc int) string {
	out := ""
	parts := strings.Split(ipStr, sep)
	for i, part := range parts {
		if i != 0 {
			out = out + sep
		}
		if part == "" {
			continue
		}
		hashVal := hashTrunc(part, salt, trunc)
		out = out + hashVal
	}
	return out
}

// Returns the hash of salt+IP.
// 1.2.3.4 will hash as hash(salt+1).hash(salt+2).hash(salt+3).hash(salt+4)
// similarly for IPv6
func hashIP(ip net.IP, salt string) string {
	ipStr := ip.String()
	if p4 := ip.To4(); len(p4) == net.IPv4len {
		return hashIPHelper(ipStr, salt, ".", 3)
	}
	if p6 := ip.To16(); len(p6) == net.IPv6len {
		return "[" + hashIPHelper(ipStr, salt, ":", 4) + "]"
	}
	return hashWord(ipStr, salt)
}

// Break the string on `/`, `.`, and `-`. Individually salt+hash each of those
// `words`. If all of the 'stuff' before the `/` looks like an IP handle it a little
// differently.
func hashString(str, salt string) string {
	// Before we break it up, if it's an IP, handle it special
	if ip := net.ParseIP(str); ip != nil {
		return hashIP(ip, salt)
	}
	out := ""
	slashParts := strings.Split(str, "/")
	for i, slashPart := range slashParts {
		if i != 0 {
			out = out + "/"
		}
		dotParts := strings.Split(slashPart, ".")
		for j, dotPart := range dotParts {
			if j != 0 {
				out = out + "."
			}
			dashParts := strings.Split(dotPart, "-")
			for k, word := range dashParts {
				if k != 0 {
					out = out + "-"
				}
				out = out + hashWord(word, salt)
			}
		}
	}
	return out
}

// check if a string can be parsed as a CIDR
func validCIDR(in string) (net.IP, *net.IPNet, bool) {
	ip, ipnet, err := net.ParseCIDR(in)
	if err != nil {
		return nil, nil, false
	}
	return ip, ipnet, true
}

func cidrHash(ip net.IP, cidr *net.IPNet, salt string) string {
	// do hash the IP portion
	out := hashIP(ip, salt)
	out = out + "/"
	// do not hash the subnet len
	ones, _ := cidr.Mask.Size()
	return fmt.Sprintf("%s%d", out, ones)
}

// SetAllowedWords allows you to specify words which will not be hashed. These will
// instead be returned unchanged.
func SetAllowedWords(allowed map[string]struct{}) {
	allowedWords = allowed
}

// SetSalt allows you to set the salt which will be used in calls to
// HashURL.
func SetSalt(salt string) {
	globalSalt = salt
}

// HashURLSalt is the same as HashURL except you can pass an explict salt to
// use for this call. Whereas HashURL uses the salt set globally.
func HashURLSalt(urlString, salt string) string {
	// If it looks like a cidr (aka 192.168.0.0/24) parse it.
	if ip, ipnet, ok := validCIDR(urlString); ok {
		return cidrHash(ip, ipnet, salt)
	}

	// Make sure that every string parses with a 'Scheme'. Stoopid RFC. Without this we
	// parse things like `127.0.0.1:8080` very oddly.
	if !strings.Contains(urlString, "://") {
		urlString = "placeholder://" + urlString
	}

	// Parse it
	u, err := url.Parse(urlString)
	if err != nil {
		// If we still don't look like a URL, just hash it and move along
		return hash(urlString, salt)
	}

	// Just print the scheme (excludig our magic string)
	out := ""
	if u.Scheme != "" && u.Scheme != "placeholder" {
		out = u.Scheme + "://"
	}

	if ip := net.ParseIP(u.Host); ip != nil {
		out = out + hashIP(ip, salt)
	} else {
		// has the hostname
		host := u.Hostname()
		if host != "" {
			out = out + hashString(host, salt)

			// hash the port
			port := u.Port()
			if port != "" {
				out = out + ":" + hashString(port, salt)
			}
		}
	}

	// hash the path
	path := u.Path
	if path != "" {
		// If the host was not found, treat the path as the host
		out = out + hashString(path, salt)
	}
	return out
}

// HashURL takes an url and returns a hash. This hash should be non-trivial to get the
// original value, but should be stable. So one can compare the output of the hash accross
// different urls. For example if openshift and com are in the 'AllowedWords' the urls
// might hash as:
//    https://this.openshift.com -> https://0a31.openshift.com
//    https://that.openshift.com -> https://deb4.openshift.com
//    https://this.that -> https://0a31.deb4
func HashURL(urlString string) string {
	return HashURLSalt(urlString, globalSalt)
}

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// GetNewSalt returns a random string of a given length. Which can be used in SetSalt.
// please note you may need to seed rand before calling this function.
func GetNewSalt(length int) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}
