package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"unicode"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseEthernetFrame(frame []byte, builder *strings.Builder) error {
	if len(frame) < 14 {
		return fmt.Errorf("invalid frame size")
	}
	destMac, sourceMac, etherType := net.HardwareAddr(frame[0:6]), net.HardwareAddr(frame[6:12]), uint16(frame[12])<<8|uint16(frame[13])

	builder.WriteString(fmt.Sprintf("eth.src %s\neth.dst %s\neth.ethertype 0x%04x\n",
		sourceMac, destMac, etherType))
	if etherType != 0x0800 {
		return nil
	}
	return parseIpv4(frame[14:], builder)
}

func parseIpv4(packet []byte, res *strings.Builder) error {
	if len(packet) < 20 {
		return fmt.Errorf("invalid ipv4 header")
	}
	version := packet[0] >> 4
	if version != 4 {
		return fmt.Errorf("invalid ipv4 version: %d", version)
	}
	headerLen := int(packet[0]&0x0f) * 4
	if headerLen < 20 || headerLen > len(packet) {
		return fmt.Errorf("invalid ipv4 header len: %d", headerLen)
	}

	totalLen := int(uint16(packet[2])<<8 | uint16(packet[3]))
	if totalLen < headerLen || totalLen > len(packet) {
		return fmt.Errorf("invali total len: %d", totalLen)
	}

	packet = packet[:totalLen]

	fragment := uint16(packet[6])<<8 | uint16(packet[7])
	fragOffset := 8 * int(fragment&0x1fff)
	mf := fragment&0x2000 != 0
	var flags []string
	if fragment&0x4000 != 0 {
		flags = append(flags, "DF")
	}
	if mf {
		flags = append(flags, "MF")
	}
	fmt.Fprintf(res, "ip.version %d\nip.ihl_bytes %d\nip.total_length %d\nip.id 0x%04x\n",
		version, headerLen, totalLen, uint16(packet[4])<<8|uint16(packet[5]))
	fmt.Fprintf(res, "ip.flags %s\nip.frag_offset %d\nip.ttl %d\nip.protocol %d\n",
		flagsf(flags), fragOffset, packet[8], packet[9])
	fmt.Fprintf(res, "ip.src %s\nip.dst %s\nip.checksum_valid %t\n",
		net.IP(packet[12:16]), net.IP(packet[16:20]), checksum(packet[:headerLen]))

	p := packet[headerLen:]
	payloadLen := len(p)
	var err error
	if fragOffset == 0 {
		if packet[9] == 6 {
			payloadLen, err = parseTcp(p, res)
		}
		if packet[9] == 17 {
			payloadLen, err = parseUdp(p, mf, res)
		}
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(res, "payload.length %d\n", payloadLen)
	return nil
}

func run() error {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("can't read data from stdin: %w", err)
	}
	frame, err := hex.DecodeString(
		strings.Map(func(r rune) rune {
			if r == ':' || unicode.IsSpace(r) /* ' ', \n, \t, etc */ {
				return -1
			}
			return r
		}, string(data)),
	)
	if err != nil {
		return fmt.Errorf("decode hex: %w", err)
	}

	var res strings.Builder
	if err := parseEthernetFrame(frame, &res); err != nil {
		return fmt.Errorf("parse frame: %w", err)
	}
	_, err = io.WriteString(os.Stdout, res.String())
	return err
}

func parseTcp(seg []byte, res *strings.Builder) (int, error) {
	if len(seg) < 20 {
		return 0, errors.New("invalid tcp segment")
	}
	headerLen := 4 * int(seg[12]>>4)
	if headerLen < 20 || headerLen > len(seg) {
		return 0, fmt.Errorf("invalid tcp header len: %d", headerLen)
	}
	var flags []string
	for b, n := range []string{"FIN", "SYN", "RST", "PSH", "ACK", "URG"} {
		if (1<<b)&seg[13] != 0 {
			flags = append(flags, n)
		}
	}

	fmt.Fprintf(res, "tcp.src_port %d\ntcp.dst_port %d\ntcp.seq %d\ntcp.ack %d\n",
		uint16(seg[0])<<8|uint16(seg[1]), uint16(seg[2])<<8|uint16(seg[3]),
		uint32(seg[4])<<24|uint32(seg[5])<<16|uint32(seg[6])<<8|uint32(seg[7]),
		uint32(seg[8])<<24|uint32(seg[9])<<16|uint32(seg[10])<<8|uint32(seg[11]))
	fmt.Fprintf(res, "tcp.data_offset_bytes %d\ntcp.flags %s\ntcp.window %d\n",
		headerLen, flagsf(flags), uint16(seg[14])<<8|uint16(seg[15]))
	return len(seg) - headerLen, nil
}

func parseUdp(datagram []byte, mf bool, res *strings.Builder) (int, error) {
	if len(datagram) < 8 {
		return 0, fmt.Errorf("invalid udp header")
	}
	length := int(uint16(datagram[4])<<8 | uint16(datagram[5]))

	if length < 8 || (!mf && length > len(datagram)) {
		return 0, fmt.Errorf("invalid udp len: %d", length)
	}
	fmt.Fprintf(res, "udp.src_port %d\nudp.dst_port %d\nudp.length %d\n",
		uint16(datagram[0])<<8|uint16(datagram[1]), uint16(datagram[2])<<8|uint16(datagram[3]), length)
	if length > len(datagram) {
		length = len(datagram)
	}
	length -= 8
	return length, nil
}

func checksum(h []byte) bool {
	var sum uint32
	for i := 0; i < len(h); i += 2 {
		sum += uint32(h[i])<<8 | uint32(h[i+1])
	}
	for sum>>16 != 0 {
		sum = (sum >> 16) + (sum & 0xffff)
	}
	return 0xffff == sum
}

func flagsf(flags []string) string {
	if len(flags) == 0 {
		return "none"
	}
	return strings.Join(flags, ",")
}
