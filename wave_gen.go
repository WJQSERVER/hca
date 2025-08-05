package hca

import (
	"encoding/binary"
	"io"
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

func (wv *stWaveHeader) Write(w io.Writer, endian binary.ByteOrder) error {
	if wv.RiffOk {
		if err := wv.Riff.Write(w, endian); err != nil {
			return err
		}
	}
	if wv.SmplOk {
		if err := wv.Smpl.Write(w, endian); err != nil {
			return err
		}
	}
	if wv.NoteOk {
		if err := wv.Note.Write(w, endian); err != nil {
			return err
		}
	}
	if wv.DataOk {
		if err := wv.Data.Write(w, endian); err != nil {
			return err
		}
	}
	return nil
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

func (h *stWAVEriff) Write(w io.Writer, endian binary.ByteOrder) error {
	if err := binary.Write(w, binary.BigEndian, h.riff); err != nil {
		return err
	}
	if err := binary.Write(w, endian, h.riffSize); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, h.wave); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, h.fmt); err != nil {
		return err
	}
	if err := binary.Write(w, endian, h.fmtSize); err != nil {
		return err
	}
	if err := binary.Write(w, endian, h.fmtType); err != nil {
		return err
	}
	if err := binary.Write(w, endian, h.fmtChannelCount); err != nil {
		return err
	}
	if err := binary.Write(w, endian, h.fmtSamplingRate); err != nil {
		return err
	}
	if err := binary.Write(w, endian, h.fmtSamplesPerSec); err != nil {
		return err
	}
	if err := binary.Write(w, endian, h.fmtSamplingSize); err != nil {
		return err
	}
	if err := binary.Write(w, endian, h.fmtBitCount); err != nil {
		return err
	}
	return nil
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

func (s *stWAVEsmpl) Write(w io.Writer, endian binary.ByteOrder) error {
	if err := binary.Write(w, binary.BigEndian, s.smpl); err != nil {
		return err
	}
	data := []interface{}{
		s.smplSize, s.manufacturer, s.product, s.samplePeriod, s.MIDIUnityNote,
		s.MIDIPitchFraction, s.SMPTEFormat, s.SMPTEOffset, s.sampleLoops,
		s.samplerData, s.loopIdentifier, s.loopType, s.loopStart, s.loopEnd,
		s.loopFraction, s.loopPlayCount,
	}
	for _, v := range data {
		if err := binary.Write(w, endian, v); err != nil {
			return err
		}
	}
	return nil
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

func (n *stWAVEnote) Write(w io.Writer, endian binary.ByteOrder) error {
	if err := binary.Write(w, binary.BigEndian, n.note); err != nil {
		return err
	}
	if err := binary.Write(w, endian, n.noteSize); err != nil {
		return err
	}
	if err := binary.Write(w, endian, n.dwName); err != nil {
		return err
	}
	if _, err := w.Write([]byte(n.comm)); err != nil {
		return err
	}
	if err := binary.Write(w, endian, byte(0)); err != nil {
		return err
	}
	padding := (4 - (len(n.comm)+1)%4) % 4
	if padding > 0 {
		if _, err := w.Write(make([]byte, padding)); err != nil {
			return err
		}
	}
	return nil
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

func (d *stWAVEdata) Write(w io.Writer, endian binary.ByteOrder) error {
	if err := binary.Write(w, binary.BigEndian, d.data); err != nil {
		return err
	}
	if err := binary.Write(w, endian, d.dataSize); err != nil {
		return err
	}
	return nil
}
