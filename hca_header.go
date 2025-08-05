package hca

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	sigMask = 0x7F7F7F7F
	sigHCA  = 0x48434100
	sigFMT  = 0x666D7400
	sigCOMP = 0x636F6D70
	sigDEC  = 0x64656300
	sigVBR  = 0x76627200
	sigATH  = 0x61746800
	sigLOOP = 0x6C6F6F70
	sigCIPH = 0x63697068
	sigRVA  = 0x72766100
	sigCOMM = 0x636F6D6D
)

func readData(r io.Reader, data interface{}) error {
	return binary.Read(r, binary.BigEndian, data)
}

func (h *Hca) loadHeader(r io.Reader) error {
	var sig uint32

	if err := readData(r, &sig); err != nil {
		return err
	}
	if sig&sigMask != sigHCA {
		return fmt.Errorf("invalid HCA signature: got %x", sig)
	}
	if err := h.hcaHeaderRead(r); err != nil {
		return err
	}

	if err := readData(r, &sig); err != nil {
		return err
	}
	if sig&sigMask != sigFMT {
		return fmt.Errorf("expected fmt signature, got %x", sig)
	}
	if err := h.fmtHeaderRead(r); err != nil {
		return err
	}

	if err := readData(r, &sig); err != nil {
		return err
	}
	if sig&sigMask == sigCOMP {
		if err := h.compHeaderRead(r); err != nil {
			return err
		}
		if err := readData(r, &sig); err != nil {
			return err
		}
	} else if sig&sigMask == sigDEC {
		if err := h.decHeaderRead(r); err != nil {
			return err
		}
		if err := readData(r, &sig); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("expected comp or dec signature, got %x", sig)
	}

	if sig&sigMask == sigVBR {
		if err := h.vbrHeaderRead(r); err != nil {
			return err
		}
		if err := readData(r, &sig); err != nil && err != io.EOF {
			return err
		}
	} else {
		h.vbrR01, h.vbrR02 = 0, 0
	}

	if sig&sigMask == sigATH {
		if err := h.athHeaderRead(r); err != nil {
			return err
		}
		if err := readData(r, &sig); err != nil && err != io.EOF {
			return err
		}
	} else {
		h.athType = 1
		if h.version >= 0x200 {
			h.athType = 0
		}
	}

	if sig&sigMask == sigLOOP {
		if err := h.loopHeaderRead(r); err != nil {
			return err
		}
		if err := readData(r, &sig); err != nil && err != io.EOF {
			return err
		}
	} else {
		h.loopFlg = false
	}

	if sig&sigMask == sigCIPH {
		if err := h.ciphHeaderRead(r); err != nil {
			return err
		}
		if err := readData(r, &sig); err != nil && err != io.EOF {
			return err
		}
	} else {
		h.ciphType = 0
	}

	if sig&sigMask == sigRVA {
		if err := h.rvaHeaderRead(r); err != nil {
			return err
		}
		if err := readData(r, &sig); err != nil && err != io.EOF {
			return err
		}
	} else {
		h.rvaVolume = 1.0
	}

	if sig&sigMask == sigCOMM {
		if err := h.commHeaderRead(r); err != nil {
			return err
		}
	} else {
		h.commLen = 0
		h.commComment = ""
	}

	if !h.ath.Init(int(h.athType), h.samplingRate) {
		return errors.New("ATH init failed")
	}
	h.cipher = NewCipher()
	if !h.cipher.Init(int(h.ciphType), h.CiphKey1, h.CiphKey2) {
		return errors.New("cipher init failed")
	}
	if h.compR03 == 0 {
		h.compR03 = 1
	}
	if !(h.compR01 == 1 && h.compR02 == 15) {
		h.compR09 = ceil2(h.compR05-(h.compR06+h.compR07), h.compR08)
	}

	h.decoder = newChannelDecoder(h.channelCount, h.compR03, h.compR04, h.compR05, h.compR06, h.compR07, h.compR08, h.compR09)
	return nil
}

func (h *Hca) hcaHeaderRead(r io.Reader) error {
	var version, dataOffset uint16
	if err := readData(r, &version); err != nil { return err }
	if err := readData(r, &dataOffset); err != nil { return err }
	h.version = uint32(version)
	h.dataOffset = uint32(dataOffset)
	return nil
}

func (h *Hca) fmtHeaderRead(r io.Reader) error {
	var raw uint32
	if err := readData(r, &raw); err != nil { return err }
	h.channelCount = (raw & 0xFF000000) >> 24
	h.samplingRate = raw & 0x00FFFFFF
	if err := readData(r, &h.blockCount); err != nil { return err }
	var r01, r02 uint16
	if err := readData(r, &r01); err != nil { return err }
	if err := readData(r, &r02); err != nil { return err }
	h.fmtR01 = uint32(r01)
	h.fmtR02 = uint32(r02)
	if !(h.channelCount >= 1 && h.channelCount <= 16) { return fmt.Errorf("invalid channel count: %d", h.channelCount) }
	if !(h.samplingRate >= 1 && h.samplingRate <= 0x7FFFFF) { return fmt.Errorf("invalid sampling rate: %d", h.samplingRate) }
	return nil
}

func (h *Hca) compHeaderRead(r io.Reader) error {
	var blockSize uint16
	if err := readData(r, &blockSize); err != nil { return err }
	h.blockSize = uint32(blockSize)

	buf := make([]byte, 10)
	if _, err := io.ReadFull(r, buf); err != nil { return err }
	h.compR01, h.compR02 = uint32(buf[0]), uint32(buf[1])
	h.compR03, h.compR04 = uint32(buf[2]), uint32(buf[3])
	h.compR05, h.compR06 = uint32(buf[4]), uint32(buf[5])
	h.compR07, h.compR08 = uint32(buf[6]), uint32(buf[7])

	if !((h.blockSize >= 8 && h.blockSize <= 0xFFFF) || (h.blockSize == 0)) { return fmt.Errorf("invalid block size: %d", h.blockSize) }
	if !(h.compR01 <= h.compR02 && h.compR02 <= 0x1F) { return fmt.Errorf("invalid comp params: r01=%d, r02=%d", h.compR01, h.compR02) }
	return nil
}

func (h *Hca) decHeaderRead(r io.Reader) error {
	var blockSize uint16
	if err := readData(r, &blockSize); err != nil { return err }
	h.blockSize = uint32(blockSize)

	buf := make([]byte, 6)
	if _, err := io.ReadFull(r, buf); err != nil { return err }
	h.compR01, h.compR02 = uint32(buf[0]), uint32(buf[1])
	h.compR03 = uint32(buf[4] & 0xF)
	h.compR04 = uint32(buf[4] >> 4)
	h.compR05 = uint32(buf[2]) + 1
	h.compR06 = uint32(buf[2]) + 1
	if buf[5] > 0 {
		h.compR06 = uint32(buf[3]) + 1
	}
	h.compR07 = h.compR05 - h.compR06
	h.compR08 = 0
	if h.compR03 == 0 { h.compR03 = 1 }

	if !((h.blockSize >= 8 && h.blockSize <= 0xFFFF) || h.blockSize == 0) { return fmt.Errorf("invalid block size: %d", h.blockSize) }
	if !(h.compR01 <= h.compR02 && h.compR02 <= 0x1F) { return fmt.Errorf("invalid comp params: r01=%d, r02=%d", h.compR01, h.compR02) }
	return nil
}

func (h *Hca) vbrHeaderRead(r io.Reader) error {
	var r01, r02 uint16
	if err := readData(r, &r01); err != nil { return err }
	if err := readData(r, &r02); err != nil { return err }
	h.vbrR01, h.vbrR02 = uint32(r01), uint32(r02)
	return nil
}

func (h *Hca) athHeaderRead(r io.Reader) error {
	var athType uint16
	if err := readData(r, &athType); err != nil { return err }
	h.athType = uint32(athType)
	return nil
}

func (h *Hca) loopHeaderRead(r io.Reader) error {
	if err := readData(r, &h.loopStart); err != nil { return err }
	if err := readData(r, &h.loopEnd); err != nil { return err }
	var r01, r02 uint16
	if err := readData(r, &r01); err != nil { return err }
	if err := readData(r, &r02); err != nil { return err }
	h.loopR01, h.loopR02 = uint32(r01), uint32(r02)
	h.loopFlg = true
	if !(h.loopStart <= h.loopEnd && h.loopEnd < h.blockCount) { return fmt.Errorf("invalid loop range") }
	return nil
}

func (h *Hca) ciphHeaderRead(r io.Reader) error {
	var ciphType uint16
	if err := readData(r, &ciphType); err != nil { return err }
	h.ciphType = uint32(ciphType)
	if !(h.ciphType == 0 || h.ciphType == 1 || h.ciphType == 0x38) { return fmt.Errorf("unsupported cipher type: %d", ciphType) }
	return nil
}

func (h *Hca) rvaHeaderRead(r io.Reader) error {
	return readData(r, &h.rvaVolume)
}

func (h *Hca) commHeaderRead(r io.Reader) error {
	var commLen byte
	if err := binary.Read(r, binary.BigEndian, &commLen); err != nil { return err }
	h.commLen = uint32(commLen)

	buf := make([]byte, h.commLen)
	if _, err := io.ReadFull(r, buf); err != nil { return err }
	h.commComment = string(buf)
	// Read trailing null byte if it exists
	if (h.commLen+1)%2 != 0 {
		var nullByte byte
		binary.Read(r, binary.BigEndian, &nullByte)
	}
	return nil
}

func ceil2(a, b uint32) uint32 {
	if b == 0 { return 0 }
	t := a / b
	if a%b != 0 { t++ }
	return t
}
