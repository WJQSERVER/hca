// Package hca an HCA (High-Compression Audio) decoder.
// This file (wave_gen.go) is responsible for building and writing the WAV file header.
package hca

import (
	"encoding/binary"
	"io"
)

// stWaveHeader 是WAV文件头的顶层结构体, 它包含了所有可能的块(chunk).
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
		SmplOk: false, // 'smpl'块默认不写入, 仅在有循环信息时激活
		NoteOk: false, // 'note'块默认不写入, 仅在有评论信息时激活
		DataOk: true,
	}
}

// Write 将完整的WAV头部信息写入指定的writer.
// 它根据 RiffOk, SmplOk 等标志决定写入哪些块.
func (wv *stWaveHeader) Write(w io.Writer, endian binary.ByteOrder) error {
	var err error
	if wv.RiffOk {
		err = wv.Riff.Write(w, endian)
		if err != nil {
			return err
		}
	}
	if wv.SmplOk {
		err = wv.Smpl.Write(w, endian)
		if err != nil {
			return err
		}
	}
	if wv.NoteOk {
		err = wv.Note.Write(w, endian)
		if err != nil {
			return err
		}
	}
	if wv.DataOk {
		err = wv.Data.Write(w, endian)
		if err != nil {
			return err
		}
	}
	return nil
}

// stWAVEriff 对应WAV文件中的 'RIFF' 和 'fmt ' 块.
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
	if err := binary.Write(w, binary.BigEndian, h.riff); err != nil { return err }
	if err := binary.Write(w, endian, h.riffSize); err != nil { return err }
	if err := binary.Write(w, binary.BigEndian, h.wave); err != nil { return err }
	if err := binary.Write(w, binary.BigEndian, h.fmt); err != nil { return err }

	if err := binary.Write(w, endian, h.fmtSize); err != nil { return err }
	if err := binary.Write(w, endian, h.fmtType); err != nil { return err }
	if err := binary.Write(w, endian, h.fmtChannelCount); err != nil { return err }
	if err := binary.Write(w, endian, h.fmtSamplingRate); err != nil { return err }
	if err := binary.Write(w, endian, h.fmtSamplesPerSec); err != nil { return err }
	if err := binary.Write(w, endian, h.fmtSamplingSize); err != nil { return err }
	if err := binary.Write(w, endian, h.fmtBitCount); err != nil { return err }

	return nil
}

// stWAVEsmpl 对应WAV文件中的 'smpl' (sample) 块, 用于定义循环信息.
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
	if err := binary.Write(w, binary.BigEndian, s.smpl); err != nil { return err }

	if err := binary.Write(w, endian, s.smplSize); err != nil { return err }
	if err := binary.Write(w, endian, s.manufacturer); err != nil { return err }
	if err := binary.Write(w, endian, s.product); err != nil { return err }
	if err := binary.Write(w, endian, s.samplePeriod); err != nil { return err }
	if err := binary.Write(w, endian, s.MIDIUnityNote); err != nil { return err }
	if err := binary.Write(w, endian, s.MIDIPitchFraction); err != nil { return err }
	if err := binary.Write(w, endian, s.SMPTEFormat); err != nil { return err }
	if err := binary.Write(w, endian, s.SMPTEOffset); err != nil { return err }
	if err := binary.Write(w, endian, s.sampleLoops); err != nil { return err }
	if err := binary.Write(w, endian, s.samplerData); err != nil { return err }
	if err := binary.Write(w, endian, s.loopIdentifier); err != nil { return err }
	if err := binary.Write(w, endian, s.loopType); err != nil { return err }
	if err := binary.Write(w, endian, s.loopStart); err != nil { return err }
	if err := binary.Write(w, endian, s.loopEnd); err != nil { return err }
	if err := binary.Write(w, endian, s.loopFraction); err != nil { return err }
	if err := binary.Write(w, endian, s.loopPlayCount); err != nil { return err }

	return nil
}

// stWAVEnote 对应WAV文件中的 'note' 块, 用于存储备注或评论信息.
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
	if err := binary.Write(w, binary.BigEndian, n.note); err != nil { return err }

	if err := binary.Write(w, endian, n.noteSize); err != nil { return err }
	if err := binary.Write(w, endian, n.dwName); err != nil { return err }

	// 写入带空终止符的字符串
	if _, err := w.Write([]byte(n.comm)); err != nil { return err }
	if err := binary.Write(w, endian, byte(0)); err != nil { return err }

	// 写入填充字节以对齐
	padding := (4 - (len(n.comm)+1)%4) % 4
	if padding > 0 {
		if _, err := w.Write(make([]byte, padding)); err != nil {
			return err
		}
	}

	return nil
}

// stWAVEdata 对应WAV文件中的 'data' 块, 它定义了实际音频数据块的开始和大小.
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
	if err := binary.Write(w, binary.BigEndian, d.data); err != nil { return err }
	if err := binary.Write(w, endian, d.dataSize); err != nil { return err }
	return nil
}
