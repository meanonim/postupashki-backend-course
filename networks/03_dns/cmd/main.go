package main

import (
	"bufio"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"
)

type reader struct {
	index int
	buf   []byte
}

func newReader() *reader {
	return &reader{
		index: 0,
		buf:   nil,
	}
}

func (r *reader) readAllAndSave(f *os.File) {
	r.buf, _ = io.ReadAll(f)
	r.index = 0
}

func (r *reader) nextString() string {
	if r.index == len(r.buf) {
		return ""
	}
	for r.index < len(r.buf) && r.buf[r.index] <= ' ' {
		r.index++
	}
	from := r.index
	for r.index < len(r.buf) && r.buf[r.index] > ' ' {
		r.index++
	}
	return string(r.buf[from:r.index])
}

var queryTypesMapping = map[string]uint{
	"A":     1,
	"NS":    2,
	"CNAME": 5,
	"TXT":   16,
	"MX":    15,
	"AAAA":  28,
}

func main() {
	os.Exit(run())
}

// yes, no error catching. why if it will be truly ok
func run() int {
	if len(os.Args) != 3 {
		return 1
	}
	ip := net.ParseIP(os.Args[1])
	port, _ := strconv.Atoi(os.Args[2])

	rdr := newReader()
	rdr.readAllAndSave(os.Stdin)
	wrt := bufio.NewWriter(os.Stdout)
	defer wrt.Flush()

	conn, _ := net.DialUDP("udp", nil, &net.UDPAddr{IP: ip, Port: port})
	defer conn.Close()

	cache := make(map[cacheK]cacheV)
	buf := make([]byte, 65536)
	for {
		name := rdr.nextString()
		if name == "" {
			break
		}
		t := rdr.nextString()
		qtype := queryTypesMapping[strings.ToUpper(t)]

		k := cacheK{
			strings.ToLower(strings.TrimSuffix(name, ".")),
			qtype,
		}
		v, ok := cache[k]
		if !ok || !time.Now().Before(v.expAt) {
			delete(cache, k)
			q := buildRequest(name, qtype)
			v.res = response(conn, q, buf)
			if v.res.status == "NOERROR" && v.res.minTTL > 0 {
				v.expAt = time.Now().Add(time.Duration(v.res.minTTL) * time.Second)
				cache[k] = v
			}
		}

		fmt.Fprintf(wrt, "query %s %s\nstatus %s\n%send\n", name, t, v.res.status, v.res.response)
		wrt.Flush()
		if v.res.status == "TIMEOUT" {
			return 1
		}
	}
	return 0
}

type cacheK struct {
	name string
	t    uint
}

type cacheV struct {
	res   dnsResponse
	expAt time.Time
}

type dnsResponse struct {
	status, response string
	minTTL           uint
}

func buildRequest(name string, qtype uint) []byte {
	r := make([]byte, 12, 271)
	rand.Read(r[:2])
	r[2] = 0x01
	r[5] = 1

	name = strings.TrimSuffix(name, ".")
	if name != "" {
		for d := range strings.SplitSeq(name, ".") {
			r = append(r, byte(len(d)))
			r = append(r, d...)
		}
	}
	r = append(r, 0, byte(qtype>>8), byte(qtype), 0, 1)
	return r
}

func response(conn *net.UDPConn, req, buffer []byte) dnsResponse {
	deadline := time.Now().Add(5 * time.Second)
	conn.SetDeadline(deadline)
	conn.Write(req)
	id := uint(req[0])<<8 | uint(req[1])
	for time.Now().Before(deadline) {
		n, _ := conn.Read(buffer)
		if n < 12 || uint(buffer[0])<<8|uint(buffer[1]) != id || buffer[2]&0x80 == 0 {
			continue
		}
		response := readResponse(buffer[:n])
		if response.status != "" {
			return response
		}
	}
	return dnsResponse{status: "TIMEOUT"}
}

func readName(msg []byte, idx int) (string, int) {
	var name strings.Builder
	var seen map[int]bool
	next := -1
	wireLength := 1
	for {
		if idx < 0 || idx >= len(msg) {
			return "", len(msg) + 1
		}
		size := int(msg[idx])
		switch size & 0xc0 {
		case 0xc0:
			if idx+2 > len(msg) {
				return "", len(msg) + 1
			}
			if next == -1 {
				next = idx + 2
			}
			pointer := int((uint(msg[idx])<<8 | uint(msg[idx+1])) & 0x3fff)
			if seen[pointer] {
				return "", len(msg) + 1
			}
			if seen == nil {
				seen = make(map[int]bool)
			}
			seen[pointer] = true
			idx = pointer
			continue
		case 0:
		default:
			return "", len(msg) + 1
		}

		idx++
		if size == 0 {
			if next == -1 {
				next = idx
			}
			if name.Len() == 0 {
				return ".", next
			}
			return name.String(), next
		}
		wireLength += size + 1
		if wireLength > 255 || idx+size > len(msg) {
			return "", len(msg) + 1
		}
		name.Write(msg[idx : idx+size])
		name.WriteByte('.')
		idx += size
	}
}

func readResponse(msg []byte) dnsResponse {
	if len(msg) < 12 {
		return dnsResponse{}
	}
	code := uint(msg[3] & 0x0f)
	statuses := [...]string{"NOERROR", "FORMERR", "SERVFAIL", "NXDOMAIN", "RCODE4", "REFUSED"}
	response := dnsResponse{status: "RCODE" + strconv.Itoa(int(code))}
	if code < uint(len(statuses)) {
		response.status = statuses[code]
	}
	idx := 12
	for count := uint(msg[4])<<8 | uint(msg[5]); count > 0; count-- {
		_, next := readName(msg, idx)
		if next+4 > len(msg) {
			return dnsResponse{}
		}
		idx = next + 4
	}

	var answers strings.Builder
	n := uint(msg[6])<<8 | uint(msg[7])
	if n > 0 {
		response.minTTL = ^uint(0)
	}
	for ; n > 0; n-- {
		_, idx = readName(msg, idx)
		if idx+10 > len(msg) {
			return dnsResponse{}
		}
		t := uint(msg[idx])<<8 | uint(msg[idx+1])
		ttl := uint(msg[idx+4])<<24 | uint(msg[idx+5])<<16 |
			uint(msg[idx+6])<<8 | uint(msg[idx+7])
		length := uint(msg[idx+8])<<8 | uint(msg[idx+9])
		idx += 10
		end := idx + int(length)
		if end > len(msg) {
			return dnsResponse{}
		}
		if ttl < response.minTTL {
			response.minTTL = ttl
		}

		typeName, v := "", ""
		switch t {
		case 28, 1:
			size := 4
			typeName = "A"
			if t == 28 {
				size, typeName = 16, "AAAA"
			}
			if end-idx != size {
				return dnsResponse{}
			}
			addr, _ := netip.AddrFromSlice(msg[idx:end])
			v = addr.String()
		case 2, 5, 15:
			typeName = "NS"
			prefix := ""
			if t == 5 {
				typeName = "CNAME"
			}
			if t == 15 {
				if end-idx < 3 {
					return dnsResponse{}
				}
				typeName = "MX"
				priority := uint(msg[idx])<<8 | uint(msg[idx+1])
				prefix = strconv.Itoa(int(priority)) + " "
				idx += 2
			}
			name, next := readName(msg, idx)
			if next != end {
				return dnsResponse{}
			}
			v = prefix + name
		case 16:
			var text strings.Builder
			for idx < end {
				size := int(msg[idx])
				idx++
				if idx+size > end {
					return dnsResponse{}
				}
				text.Write(msg[idx : idx+size])
				idx += size
			}
			typeName, v = "TXT", text.String()
		}
		if typeName != "" {
			fmt.Fprintf(&answers, "answer %s %s %d\n", typeName, v, ttl)
		}
		idx = end
	}
	response.response = answers.String()
	return response
}
