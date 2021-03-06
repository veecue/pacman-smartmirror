package packet

import (
	"errors"
	"fmt"
	"regexp"
)

var (
	filenameRegex = regexp.MustCompile(`(.+)-(.+-.+)-(.+)\.pkg\.tar\.(xz|zst)`)
)

// Packet represents a pacman Packet
type Packet struct {
	Name        string
	Version     string
	Arch        string
	Compression string
}

// Filename returns the corresponding filename the packet is saved as
func (p *Packet) Filename() string {
	return fmt.Sprintf("%s-%s-%s.pkg.tar.%s",
		p.Name,
		p.Version,
		p.Arch,
		p.Compression,
	)
}

// FromFilename parses a packet's filename and returns the parsed information
func FromFilename(filename string) (*Packet, error) {
	matches := filenameRegex.FindStringSubmatch(filename)
	if len(matches) != 5 {
		return nil, errors.New("Invalid filename")
	}

	return &Packet{
		Name:        matches[1],
		Version:     matches[2],
		Arch:        matches[3],
		Compression: matches[4],
	}, nil
}
