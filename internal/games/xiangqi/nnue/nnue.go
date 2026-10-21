// Package nnue implements the Pikafish 2024 HalfKAv2_hm NNUE evaluator in pure Go.
package nnue

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
)

const (
	version       = 0x7AF32F20
	ftHalf        = 1024
	psqtBuckets   = 8
	layerStacks   = 8
	inputDims     = 38880
	psqPlanes     = 8 * 90
	outputScale   = 16
	weightScale   = 6
	fc0Outputs    = 15
	fc1Outputs    = 32
	ftHash        = 0xd17b100 ^ (ftHalf * 2)
	lebMagic      = "COMPRESSED_LEB128"
	featureName   = "HalfKAv2_hm"
	outputDivisor = 127 * (1 << weightScale)
)

// Network is a loaded Pikafish NNUE network.
type Network struct {
	bias       []int16
	weights    []int16 // [feature][1024]
	psqt       []int32 // [feature][8]
	layers     [layerStacks]stack
	desc       string
	fileSize   int64
	featureLen int
}

type layer struct {
	bias    []int32
	weights []int8 // row-major [output][input]
	in, out int
}

type stack struct {
	fc0 layer
	fc1 layer
	fc2 layer
}

// Load reads a Pikafish 2024 .nnue file.
func Load(path string) (*Network, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	p := &parser{data: data}
	if p.u32() != version {
		return nil, errors.New("nnue: unsupported version")
	}
	_ = p.u32() // architecture hash; sections carry their own hashes
	descLen := p.u32()
	if descLen > uint32(len(data)) {
		return nil, errors.New("nnue: invalid description length")
	}
	desc := string(p.bytes(int(descLen)))
	if p.err != nil {
		return nil, p.err
	}

	n := &Network{desc: desc, fileSize: int64(len(data)), featureLen: inputDims}
	if got := p.u32(); got != ftHash {
		return nil, fmt.Errorf("nnue: feature hash %#x, want %#x", got, ftHash)
	}
	n.bias = p.lebInt16(ftHalf)
	if p.err != nil {
		return nil, fmt.Errorf("nnue: bias: %w", p.err)
	}
	n.weights = p.lebInt16(ftHalf * inputDims)
	if p.err != nil {
		return nil, fmt.Errorf("nnue: weights: %w", p.err)
	}
	n.psqt = p.lebInt32(psqtBuckets * inputDims)
	if p.err != nil {
		return nil, fmt.Errorf("nnue: psqt: %w", p.err)
	}

	for i := 0; i < layerStacks; i++ {
		_ = p.u32() // network hash
		n.layers[i].fc0 = p.layer(fc0Outputs+1, ftHalf)
		n.layers[i].fc1 = p.layer(fc1Outputs, fc1Inputs)
		n.layers[i].fc2 = p.layer(1, fc1Outputs)
		if p.err != nil {
			return nil, fmt.Errorf("nnue: layer %d: %w", i, p.err)
		}
	}
	if p.off != len(data) {
		return nil, fmt.Errorf("nnue: %d trailing bytes", len(data)-p.off)
	}
	return n, nil
}

// Description returns the string embedded in the network header.
func (n *Network) Description() string { return n.desc }

// FileSize returns the loaded file size in bytes.
func (n *Network) FileSize() int64 { return n.fileSize }

func (p *parser) layer(out, in int) layer {
	return layer{bias: p.int32s(out), weights: p.int8s(out * in), in: in, out: out}
}

// Evaluate evaluates a 90-square board. Cells are "rR", "bK", ... or "".
// turn is "w" for red to move and "b" for black to move.
func (n *Network) Evaluate(board []string, turn string) (int, error) {
	if len(board) != 90 {
		return 0, fmt.Errorf("nnue: board has %d cells, want 90", len(board))
	}
	pieces := make([]int, 90)
	for i, cell := range board {
		if cell == "" {
			continue
		}
		code, ok := pieceCode(cell)
		if !ok {
			return 0, fmt.Errorf("nnue: invalid piece %q at %d", cell, i)
		}
		pieces[i] = code
	}
	acc, err := n.NewAccumulator(pieces)
	if err != nil {
		return 0, err
	}
	return n.EvaluateAccumulator(acc, turn)
}
func kingAndAB(pieces []int, perspective int) (int, int) {
	king := -1
	a, b := 0, 0
	for sq, pc := range pieces {
		if pc == 0 {
			continue
		}
		isRed := pc < 8
		if (perspective == 0) != isRed {
			continue
		}
		switch pieceType(pc) {
		case "K":
			king = sq
		case "A":
			a++
		case "B":
			b++
		}
	}
	return king, a*3 + b
}

func pieceCode(cell string) (int, bool) {
	if len(cell) < 2 {
		return 0, false
	}
	base := 8
	if cell[0] == 'r' {
		base = 0
	}
	var code int
	switch cell[1] {
	case 'R':
		code = 1
	case 'A':
		code = 2
	case 'C':
		code = 3
	case 'P':
		code = 4
	case 'N':
		code = 5
	case 'B':
		code = 6
	case 'K':
		code = 7
	default:
		return 0, false
	}
	return base + code, true
}

func pieceType(pc int) string {
	switch pc % 8 {
	case 1:
		return "R"
	case 2:
		return "A"
	case 3:
		return "C"
	case 4:
		return "P"
	case 5:
		return "N"
	case 6:
		return "B"
	case 7:
		return "K"
	}
	return ""
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// parser reads the little-endian, signed-LEB128 Pikafish format.
type parser struct {
	data []byte
	off  int
	err  error
}

func (p *parser) bytes(n int) []byte {
	if p.err != nil {
		return nil
	}
	if n < 0 || p.off+n > len(p.data) {
		p.err = errors.New("nnue: unexpected end of file")
		return nil
	}
	b := p.data[p.off : p.off+n]
	p.off += n
	return b
}

func (p *parser) u32() uint32 {
	b := p.bytes(4)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint32(b)
}

func (p *parser) int32s(n int) []int32 {
	b := p.bytes(n * 4)
	if b == nil {
		return nil
	}
	out := make([]int32, n)
	for i := range out {
		out[i] = int32(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return out
}

func (p *parser) int8s(n int) []int8 {
	b := p.bytes(n)
	if b == nil {
		return nil
	}
	out := make([]int8, n)
	for i := range out {
		out[i] = int8(b[i])
	}
	return out
}

func (p *parser) lebInt16(n int) []int16 {
	vals := p.leb(n)
	out := make([]int16, n)
	for i, v := range vals {
		out[i] = int16(v)
	}
	return out
}

func (p *parser) lebInt32(n int) []int32 {
	vals := p.leb(n)
	out := make([]int32, n)
	for i, v := range vals {
		out[i] = int32(v)
	}
	return out
}

func (p *parser) leb(n int) []int64 {
	magic := p.bytes(len(lebMagic))
	if p.err != nil {
		return nil
	}
	if string(magic) != lebMagic {
		p.err = fmt.Errorf("nnue: bad compression marker")
		return nil
	}
	size := p.u32()
	if p.err != nil {
		return nil
	}
	raw := p.bytes(int(size))
	if p.err != nil {
		return nil
	}
	out := make([]int64, n)
	pos := 0
	for i := 0; i < n; i++ {
		var value int64
		var shift uint
		for {
			if pos >= len(raw) {
				p.err = errors.New("nnue: truncated LEB128 data")
				return nil
			}
			b := raw[pos]
			pos++
			value |= int64(b&0x7f) << shift
			shift += 7
			if b&0x80 == 0 {
				if shift < 64 && b&0x40 != 0 {
					value |= ^int64(0) << shift
				}
				break
			}
			if shift > 63 {
				p.err = errors.New("nnue: invalid LEB128 value")
				return nil
			}
		}
		out[i] = value
	}
	if pos != len(raw) {
		p.err = fmt.Errorf("nnue: %d unused LEB128 bytes", len(raw)-pos)
		return nil
	}
	return out
}
