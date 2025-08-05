package hca

import (
	"encoding/binary"
	"io"

	"github.com/vazrupe/endibuf"
)

type stWaveHeader struct {
	Riff *stWAVEriff
	Smpl *stWAVEsmpl
	Note *stWAVEnote
	Data *stWAVEdata

	RiffOk bool
	SmplOk bool
	NoteOk bool
	DataOk bool
}

func newWaveHeader() *stWaveHeader {
	return &stWaveHeader{
		Riff: newWaveRiff(),
		Smpl: newWaveSmpl(),
		Note: newWaveNote(),
		Data: newWaveData(),

		RiffOk: true,
		SmplOk: false,
		NoteOk: false,
		DataOk: true,
	}
}

func (wv *stWaveHeader) Write(w *endibuf.Writer) {
	if wv.RiffOk {
		wv.Riff.Write(w)
	}
	if wv.SmplOk {
		wv.Smpl.Write(w)
	}
	if wv.NoteOk {
		wv.Note.Write(w)
	}
	if wv.DataOk {
		wv.Data.Write(w)
	}
}

func (wv *stWaveHeader) NeoWrite(w io.Writer, endian binary.ByteOrder) {
	if wv.RiffOk {
		wv.Riff.NeoWrite(w, endian)
	}
	if wv.SmplOk {
		wv.Smpl.NeoWrite(w, endian)
	}
	if wv.NoteOk {
		wv.Note.NeoWrite(w, endian)
	}
	if wv.DataOk {
		wv.Data.NeoWrite(w, endian)
	}
}

type stWAVEriff struct {
	riff             []byte
	riffSize         uint32
	wave             []byte
	fmt              []byte
	fmtSize          uint32
	fmtType          uint16
	fmtChannelCount  uint16
	fmtSamplingRate  uint32
	fmtSamplesPerSec uint32
	fmtSamplingSize  uint16
	fmtBitCount      uint16
}

func newWaveRiff() *stWAVEriff {
	return &stWAVEriff{
		riff:             []byte{'R', 'I', 'F', 'F'},
		wave:             []byte{'W', 'A', 'V', 'E'},
		fmt:              []byte{'f', 'm', 't', ' '},
		fmtSize:          0x10,
	}
}

func (h *stWAVEriff) Write(w *endibuf.Writer) {
	endianSave := w.Endian
	w.Endian = binary.BigEndian
	w.WriteBytes(h.riff)
	w.Endian = binary.LittleEndian
	w.WriteUint32(h.riffSize)
	w.Endian = binary.BigEndian
	w.WriteBytes(h.wave)
	w.WriteBytes(h.fmt)
	w.Endian = binary.LittleEndian
	w.WriteUint32(h.fmtSize)
	w.WriteUint16(h.fmtType)
	w.WriteUint16(h.fmtChannelCount)
	w.WriteUint32(h.fmtSamplingRate)
	w.WriteUint32(h.fmtSamplesPerSec)
	w.WriteUint16(h.fmtSamplingSize)
	w.WriteUint16(h.fmtBitCount)
	w.Endian = endianSave
}

// NeoWrite writes the RIFF header to an io.Writer with specified endianness.
// NeoWrite 将 RIFF 头部以指定的字节序写入一个 io.Writer.
func (h *stWAVEriff) NeoWrite(w io.Writer, endian binary.ByteOrder) {
	binary.Write(w, binary.BigEndian, h.riff)
	binary.Write(w, endian, h.riffSize)
	binary.Write(w, binary.BigEndian, h.wave)
	binary.Write(w, binary.BigEndian, h.fmt)
	binary.Write(w, endian, h.fmtSize)
	binary.Write(w, endian, h.fmtType)
	binary.Write(w, endian, h.fmtChannelCount)
	binary.Write(w, endian, h.fmtSamplingRate)
	binary.Write(w, endian, h.fmtSamplesPerSec)
	binary.Write(w, endian, h.fmtSamplingSize)
	binary.Write(w, endian, h.fmtBitCount)
}

type stWAVEsmpl struct {
	smpl              []byte
	smplSize          uint32
	manufacturer      uint32
	product           uint32
	samplePeriod      uint32
	MIDIUnityNote     uint32
	MIDIPitchFraction uint32
	SMPTEFormat       uint32
	SMPTEOffset       uint32
	sampleLoops       uint32
	samplerData       uint32
	loopIdentifier    uint32
	loopType          uint32
	loopStart         uint32
	loopEnd           uint32
	loopFraction      uint32
	loopPlayCount     uint32
}

func newWaveSmpl() *stWAVEsmpl {
	return &stWAVEsmpl{
		smpl:              []byte{'s', 'm', 'p', 'l'},
		smplSize:          0x3C,
		MIDIUnityNote:     0x3C,
		sampleLoops:       1,
		samplerData:       0x18,
	}
}

func (s *stWAVEsmpl) Write(w *endibuf.Writer) {
	endianSave := w.Endian
	w.Endian = binary.BigEndian
	w.WriteBytes(s.smpl)
	w.Endian = binary.LittleEndian
	w.WriteUint32(s.smplSize)
	w.WriteUint32(s.manufacturer)
	w.WriteUint32(s.product)
	w.WriteUint32(s.samplePeriod)
	w.WriteUint32(s.MIDIUnityNote)
	w.WriteUint32(s.MIDIPitchFraction)
	w.WriteUint32(s.SMPTEFormat)
	w.WriteUint32(s.SMPTEOffset)
	w.WriteUint32(s.sampleLoops)
	w.WriteUint32(s.samplerData)
	w.WriteUint32(s.loopIdentifier)
	w.WriteUint32(s.loopType)
	w.WriteUint32(s.loopStart)
	w.WriteUint32(s.loopEnd)
	w.WriteUint32(s.loopFraction)
	w.WriteUint32(s.loopPlayCount)
	w.Endian = endianSave
}

// NeoWrite writes the smpl chunk to an io.Writer with specified endianness.
// NeoWrite 将 smpl 区块以指定的字节序写入一个 io.Writer.
func (s *stWAVEsmpl) NeoWrite(w io.Writer, endian binary.ByteOrder) {
	binary.Write(w, binary.BigEndian, s.smpl)
	binary.Write(w, endian, s.smplSize)
	binary.Write(w, endian, s.manufacturer)
	binary.Write(w, endian, s.product)
	binary.Write(w, endian, s.samplePeriod)
	binary.Write(w, endian, s.MIDIUnityNote)
	binary.Write(w, endian, s.MIDIPitchFraction)
	binary.Write(w, endian, s.SMPTEFormat)
	binary.Write(w, endian, s.SMPTEOffset)
	binary.Write(w, endian, s.sampleLoops)
	binary.Write(w, endian, s.samplerData)
	binary.Write(w, endian, s.loopIdentifier)
	binary.Write(w, endian, s.loopType)
	binary.Write(w, endian, s.loopStart)
	binary.Write(w, endian, s.loopEnd)
	binary.Write(w, endian, s.loopFraction)
	binary.Write(w, endian, s.loopPlayCount)
}

type stWAVEnote struct {
	note     []byte
	noteSize uint32
	dwName   uint32
	comm     string
}

func newWaveNote() *stWAVEnote {
	return &stWAVEnote{
		note: []byte{'n', 'o', 't', 'e'},
	}
}

func (n *stWAVEnote) Write(w *endibuf.Writer) {
	endianSave := w.Endian
	w.Endian = binary.BigEndian
	w.WriteBytes(n.note)
	w.Endian = binary.LittleEndian
	w.WriteUint32(n.noteSize)
	w.WriteUint32(n.dwName)
	w.WriteCString(n.comm)
	w.Endian = endianSave
}

// NeoWrite writes the note chunk to an io.Writer with specified endianness.
// NeoWrite 将 note 区块以指定的字节序写入一个 io.Writer.
func (n *stWAVEnote) NeoWrite(w io.Writer, endian binary.ByteOrder) {
	binary.Write(w, binary.BigEndian, n.note)
	binary.Write(w, endian, n.noteSize)
	binary.Write(w, endian, n.dwName)
	binary.Write(w, endian, []byte(n.comm))
	binary.Write(w, endian, byte(0))
}

type stWAVEdata struct {
	data     []byte
	dataSize uint32
}

func newWaveData() *stWAVEdata {
	return &stWAVEdata{
		data: []byte{'d', 'a', 't', 'a'},
	}
}

func (d *stWAVEdata) Write(w *endibuf.Writer) {
	endianSave := w.Endian
	w.Endian = binary.BigEndian
	w.WriteBytes(d.data)
	w.Endian = binary.LittleEndian
	w.WriteUint32(d.dataSize)
	w.Endian = endianSave
}

// NeoWrite writes the data chunk header to an io.Writer with specified endianness.
// NeoWrite 将 data 区块的头部以指定的字节序写入一个 io.Writer.
func (d *stWAVEdata) NeoWrite(w io.Writer, endian binary.ByteOrder) {
	binary.Write(w, binary.BigEndian, d.data)
	binary.Write(w, endian, d.dataSize)
}
