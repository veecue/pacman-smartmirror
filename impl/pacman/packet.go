package pacman

import (
	"fmt"
	"regexp"

	"github.com/veecue/pacman-smartmirror/impl"
	"github.com/veecue/pacman-smartmirror/packet"
)

var (
	filenameRegex = regexp.MustCompile(`^(.+)-(.+-.+)-(.+)\.pkg\.tar\.(xz|zst)$`)
)

// pkg represents a pacman pkg
type pkg struct {
	name        string
	version     string
	arch        string
	compression string
}

var _ packet.Packet = &pkg{}

func (p *pkg) Version() string {
	return p.version
}

func (p *pkg) Name() string {
	return p.name
}

// Filename returns the corresponding filename the packet is saved as
func (p *pkg) Filename() string {
	return fmt.Sprintf("%s-%s-%s.pkg.tar.%s",
		p.name,
		p.version,
		p.arch,
		p.compression,
	)
}

// PacketFromFilename parses a packet's filename and returns the parsed information
func (*pacmanImpl) PacketFromFilename(filename string) (packet.Packet, error) {
	matches := filenameRegex.FindStringSubmatch(filename)
	if len(matches) != 5 {
		return nil, impl.ErrInvalidFilename
	}

	return &pkg{
		name:        matches[1],
		version:     matches[2],
		arch:        matches[3],
		compression: matches[4],
	}, nil
}