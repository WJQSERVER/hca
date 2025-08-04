// Package hca 实现了对HCA (High-Compression Audio) 格式的解码.
// 这个文件 (decoder.go) 包含了所有的核心解码逻辑, 包括头部解析, 数据块解密,
// MDCT反变换, 以及最终输出为WAV格式的全部过程.
package hca

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// --- 公开API (Public API) ---

// DecodeFromFile 是一个便捷函数, 用于从指定路径解码HCA文件, 并将输出的WAV数据保存到目标路径.
// 这个函数会处理文件的打开和关闭, 并在解码失败时清理掉不完整的目标文件.
func (h *Hca) DecodeFromFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}

	err = h.Decode(srcFile, dstFile)

	if err != nil {
		dstFile.Close()
		os.Remove(dst) // 解码失败时删除不完整的文件
		return fmt.Errorf("decoding failed: %w", err)
	}

	// 确保所有内容都写入磁盘
	if err := dstFile.Sync(); err != nil {
		dstFile.Close()
		return fmt.Errorf("failed to sync destination file: %w", err)
	}

	if err := dstFile.Close(); err != nil {
		return fmt.Errorf("failed to close destination file: %w", err)
	}

	return nil
}

// DecodeFromBytes 是一个便捷函数, 用于从内存中的字节切片解码HCA数据.
// 它返回一个包含WAV数据的新的字节切片, 或在失败时返回错误.
func (h *Hca) DecodeFromBytes(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("input data is empty")
	}

	reader := bytes.NewReader(data)
	writer := &bytes.Buffer{}

	if err := h.Decode(reader, writer); err != nil {
		return nil, err
	}

	return writer.Bytes(), nil
}

// Decode 是核心的解码函数. 它从一个 io.ReadSeeker 流式解码HCA数据, 并将输出的WAV数据写入一个 io.Writer.
// 这是所有解码操作的基础, 提供了最大的灵活性.
func (h *Hca) Decode(r io.ReadSeeker, w io.Writer) error {
	// 1. 验证用户配置的解码模式
	switch h.Mode {
	case ModeFloat, Mode8Bit, Mode16Bit, Mode24Bit, Mode32Bit:
		// valid mode
	default:
		return fmt.Errorf("invalid output mode: %d", h.Mode)
	}

	// 2. 读取并解析HCA文件头信息
	if err := h.loadHeader(r); err != nil {
		return fmt.Errorf("failed to load HCA header: %w", err)
	}

	// 3. 基于解析出的声道数等信息, 初始化用于内存优化的sync.Pool
	h.initPools()

	// 4. 将读取指针移动到HCA数据区的起始位置
	if _, err := r.Seek(int64(h.dataOffset), io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek to data offset: %w", err)
	}

	// 5. 构建WAV文件头并写入输出流
	wavHeader := h.buildWaveHeader()
	if err := wavHeader.Write(w, binary.LittleEndian); err != nil {
		return fmt.Errorf("failed to write WAV header: %w", err)
	}

	// 6. 结合文件自带的RVA音量和用户指定的音量
	h.rvaVolume *= h.Volume

	// 7. 循环解码所有数据块
	var err error
	if h.Loop == 0 { // 无循环或使用文件内循环设置
		err = h.decodeBlocks(r, w, h.dataOffset, h.blockCount)
	} else { // 用户强制指定循环次数
		loopBlockOffset := h.dataOffset + h.loopStart*h.blockSize
		loopBlockCount := h.loopEnd - h.loopStart

		// 解码直到循环结束点
		err = h.decodeBlocks(r, w, h.dataOffset, h.loopEnd)
		if err != nil {
			return err
		}
		// 根据用户次数循环解码循环部分
		for i := 1; i < h.Loop && err == nil; i++ {
			err = h.decodeBlocks(r, w, loopBlockOffset, loopBlockCount)
		}
		if err != nil {
			return err
		}
		// 解码文件尾部 (从循环点到文件末尾)
		err = h.decodeBlocks(r, w, loopBlockOffset, h.blockCount-h.loopStart)
	}

	return err
}

// initPools 根据解码出的声道数和块大小, 初始化sync.Pool.
func (h *Hca) initPools() {
	// 每个HCA块总是解码出 8 * 128 = 1024 个采样点
	samplesPerBlock := 1024 * int(h.channelCount)

	h.float32Pool.New = func() interface{} {
		return make([]float32, samplesPerBlock)
	}
	h.int8Pool.New = func() interface{} {
		return make([]uint8, samplesPerBlock)
	}
	h.int16Pool.New = func() interface{} {
		return make([]int16, samplesPerBlock)
	}
	h.byte24Pool.New = func() interface{} {
		return make([]byte, samplesPerBlock*3)
	}
	h.int32Pool.New = func() interface{} {
		return make([]int32, samplesPerBlock)
	}
}

// --- 核心解码逻辑 (Core Decoding Logic) ---

// decodeBlocks 从 reader 读取指定数量的块, 解码并写入 writer.
func (h *Hca) decodeBlocks(r io.ReadSeeker, w io.Writer, address, count uint32) error {
	if _, err := r.Seek(int64(address), io.SeekStart); err != nil {
		return fmt.Errorf("seek to block address %d failed: %w", address, err)
	}

	blockBuffer := make([]byte, h.blockSize)

	for l := uint32(0); l < count; l++ {
		// 读取一整个块的数据
		if _, err := io.ReadFull(r, blockBuffer); err != nil {
			return fmt.Errorf("failed to read block %d: %w", l, err)
		}

		// 解码单个块
		if err := h.decode(blockBuffer); err != nil {
			return fmt.Errorf("failed to decode block %d: %w", l, err)
		}

		// 序列化 (获取解码后的 PCM float32 数据)
		saveBlock := h.decoder.waveSerialize(h.rvaVolume)

		// 转换并保存到 writer
		if err := h.save(saveBlock, w, binary.LittleEndian); err != nil {
			return fmt.Errorf("failed to save block %d: %w", l, err)
		}
	}
	return nil
}

// decode 解码单个HCA数据块.
func (h *Hca) decode(data []byte) error {
	if len(data) < int(h.blockSize) {
		return fmt.Errorf("insufficient data for a block, expected %d, got %d", h.blockSize, len(data))
	}
	if checkSum(data, 0) != 0 {
		// 在某些情况下, 校验和错误可能是可接受的, 但这里我们作为错误处理
		return fmt.Errorf("block checksum mismatch")
	}

	// 解密数据块
	mask := h.cipher.Mask(data)
	d := &clData{}
	d.Init(mask, int(h.blockSize))

	magic := d.GetBit(16)
	if magic != 0xFFFF {
		// 块起始魔数不匹配, 可能表示数据损坏或非标准HCA
		return fmt.Errorf("invalid block magic number, expected 0xFFFF, got 0x%X", magic)
	}

	// 核心解码
	h.decoder.decode(d, h.ath.GetTable())

	return nil
}

// checkSum 计算给定数据的CRC16校验和.
func checkSum(data []byte, sum uint16) uint16 {
	// CRC16-CCITT a001
	res := sum
	v := []uint16{
		0x0000, 0x8005, 0x800F, 0x000A, 0x801B, 0x001E, 0x0014, 0x8011, 0x8033, 0x0036, 0x003C, 0x8039, 0x0028, 0x802D, 0x8027, 0x0022,
		0x8063, 0x0066, 0x006C, 0x8069, 0x0078, 0x807D, 0x8077, 0x0072, 0x0050, 0x8055, 0x805F, 0x005A, 0x804B, 0x004E, 0x0044, 0x8041,
		0x80C3, 0x00C6, 0x00CC, 0x80C9, 0x00D8, 0x80DD, 0x80D7, 0x00D2, 0x00F0, 0x80F5, 0x80FF, 0x00FA, 0x80EB, 0x00EE, 0x00E4, 0x80E1,
		0x00A0, 0x80A5, 0x80AF, 0x00AA, 0x80BB, 0x00BE, 0x00B4, 0x80B1, 0x8093, 0x0096, 0x009C, 0x8099, 0x0088, 0x808D, 0x8087, 0x0082,
		0x8183, 0x0186, 0x018C, 0x8189, 0x0198, 0x819D, 0x8197, 0x0192, 0x01B0, 0x81B5, 0x81BF, 0x01BA, 0x81AB, 0x01AE, 0x01A4, 0x81A1,
		0x01E0, 0x81E5, 0x81EF, 0x01EA, 0x81FB, 0x01FE, 0x01F4, 0x81F1, 0x81D3, 0x01D6, 0x01DC, 0x81D9, 0x01C8, 0x81CD, 0x81C7, 0x01C2,
		0x0140, 0x8145, 0x814F, 0x014A, 0x815B, 0x015E, 0x0154, 0x8151, 0x8173, 0x0176, 0x017C, 0x8179, 0x0168, 0x816D, 0x8167, 0x0162,
		0x8123, 0x0126, 0x012C, 0x8129, 0x0138, 0x813D, 0x8137, 0x0132, 0x0110, 0x8115, 0x811F, 0x011A, 0x810B, 0x010E, 0x0104, 0x8101,
		0x8303, 0x0306, 0x030C, 0x8309, 0x0318, 0x831D, 0x8317, 0x0312, 0x0330, 0x8335, 0x833F, 0x033A, 0x832B, 0x032E, 0x0324, 0x8321,
		0x0360, 0x8365, 0x836F, 0x036A, 0x837B, 0x037E, 0x0374, 0x8371, 0x8353, 0x0356, 0x035C, 0x8359, 0x0348, 0x834D, 0x8347, 0x0342,
		0x03C0, 0x83C5, 0x83CF, 0x03CA, 0x83DB, 0x03DE, 0x03D4, 0x83D1, 0x83F3, 0x03F6, 0x03FC, 0x83F9, 0x03E8, 0x83ED, 0x83E7, 0x03E2,
		0x83A3, 0x03A6, 0x03AC, 0x83A9, 0x03B8, 0x83BD, 0x83B7, 0x03B2, 0x0390, 0x8395, 0x839F, 0x039A, 0x838B, 0x038E, 0x0384, 0x8381,
		0x0280, 0x8285, 0x828F, 0x028A, 0x829B, 0x029E, 0x0294, 0x8291, 0x82B3, 0x02B6, 0x02BC, 0x82B9, 0x02A8, 0x82AD, 0x82A7, 0x02A2,
		0x82E3, 0x02E6, 0x02EC, 0x82E9, 0x02F8, 0x82FD, 0x82F7, 0x02F2, 0x02D0, 0x82D5, 0x82DF, 0x02DA, 0x82CB, 0x02CE, 0x02C4, 0x82C1,
		0x8243, 0x0246, 0x024C, 0x8249, 0x0258, 0x825D, 0x8257, 0x0252, 0x0270, 0x8275, 0x827F, 0x027A, 0x826B, 0x026E, 0x0264, 0x8261,
		0x0220, 0x8225, 0x822F, 0x022A, 0x823B, 0x023E, 0x0234, 0x8231, 0x8213, 0x0216, 0x021C, 0x8219, 0x0208, 0x820D, 0x8207, 0x0202,
	}
	for i := 0; i < len(data); i++ {
		res = (res << 8) ^ v[byte(res>>8)^data[i]]
	}
	return res
}

// --- WAV输出与采样转换 (Wave Output & Sample Conversion) ---

// save 将解码后的float32样本数据转换为目标格式并写入writer.
func (h *Hca) save(base []float32, w io.Writer, endian binary.ByteOrder) error {
	var err error
	switch h.Mode {
	case ModeFloat:
		// 在浮点模式下, base可以直接写入, 无需通过池
		err = WriteData(base, w, endian)
	case Mode8Bit:
		buf := h.int8Pool.Get().([]uint8)
		mode8BitConvert(buf, base)
		err = WriteData(buf, w, endian)
		h.int8Pool.Put(buf)
	case Mode16Bit:
		buf := h.int16Pool.Get().([]int16)
		mode16BitConvert(buf, base)
		err = WriteData(buf, w, endian)
		h.int16Pool.Put(buf)
	case Mode24Bit:
		buf := h.byte24Pool.Get().([]byte)
		mode24BitConvert(buf, base)
		_, err = w.Write(buf)
		h.byte24Pool.Put(buf)
	case Mode32Bit:
		buf := h.int32Pool.Get().([]int32)
		mode32BitConvert(buf, base)
		err = WriteData(buf, w, endian)
		h.int32Pool.Put(buf)
	}
	return err
}

// WriteData 将各种数字类型或其切片以指定的字节序写入writer.
// 针对切片类型进行了优化, 会一次性写入整个切片, 而不是逐个元素写入.
func WriteData(data interface{}, w io.Writer, endian binary.ByteOrder) error {
	return binary.Write(w, endian, data)
}

// --- 格式转换函数 ---

func mode8BitConvert(dst []uint8, src []float32) {
	for i := range dst {
		// 转换为 8 位无符号整数, 范围 0-255
		val := int(src[i]*127) + 128
		if val > 255 {
			val = 255
		}
		if val < 0 {
			val = 0
		}
		dst[i] = uint8(val)
	}
}

func mode16BitConvert(dst []int16, src []float32) {
	for i := range dst {
		val := src[i] * 32767.0
		if val > 32767.0 {
			val = 32767.0
		}
		if val < -32768.0 {
			val = -32768.0
		}
		dst[i] = int16(val)
	}
}

func mode24BitConvert(dst []byte, src []float32) {
	for i := range src {
		val := int32(src[i] * 8388607.0) // 0x7FFFFF
		if val > 8388607 {
			val = 8388607
		}
		if val < -8388608 {
			val = -8388608
		}
		// 小端序写入
		dst[i*3+0] = byte(val)
		dst[i*3+1] = byte(val >> 8)
		dst[i*3+2] = byte(val >> 16)
	}
}

func mode32BitConvert(dst []int32, src []float32) {
	for i := range dst {
		val := src[i] * 2147483647.0 // 0x7FFFFFFF
		if val > 2147483647.0 {
			val = 2147483647.0
		}
		if val < -2147483648.0 {
			val = -2147483648.0
		}
		dst[i] = int32(val)
	}
}

// buildWaveHeader 构建 WAV 文件头.
func (h *Hca) buildWaveHeader() *stWaveHeader {
	wavHeader := newWaveHeader()

	riff := wavHeader.Riff
	smpl := wavHeader.Smpl
	note := wavHeader.Note
	data := wavHeader.Data

	if h.Mode > 0 {
		riff.fmtType = 1 // PCM
		riff.fmtBitCount = uint16(h.Mode)
	} else {
		riff.fmtType = 3 // IEEE Float
		riff.fmtBitCount = 32
	}
	riff.fmtChannelCount = uint16(h.channelCount)
	riff.fmtSamplingRate = h.samplingRate
	riff.fmtSamplingSize = riff.fmtBitCount / 8 * riff.fmtChannelCount
	riff.fmtSamplesPerSec = riff.fmtSamplingRate * uint32(riff.fmtSamplingSize)

	if h.loopFlg {
		wavHeader.SmplOk = true
		smpl.samplePeriod = uint32(1.0 / float64(riff.fmtSamplingRate) * 1000000000)
		smpl.loopStart = h.loopStart * 0x80 * 8
		smpl.loopEnd = h.loopEnd * 0x80 * 8
		if h.loopR01 == 0x80 { // 无限循环
			smpl.loopPlayCount = 0
		} else {
			smpl.loopPlayCount = h.loopR01
		}
	}

	if h.commLen > 0 {
		wavHeader.NoteOk = true
		note.noteSize = 4 + h.commLen + 1
		note.comm = h.commComment
		if (note.noteSize & 3) != 0 {
			note.noteSize += 4 - (note.noteSize & 3)
		}
	}

	totalSamples := h.blockCount * 0x80 * 8
	if h.Loop > 0 {
		loopSamples := (h.loopEnd - h.loopStart) * 0x80 * 8
		totalSamples += loopSamples * uint32(h.Loop-1)
	}
	data.dataSize = totalSamples * uint32(riff.fmtSamplingSize)

	riff.riffSize = 0x24 + data.dataSize // 0x24 is the size of the header up to 'data' chunk
	if wavHeader.SmplOk {
		riff.riffSize += 0x3C // Size of smpl chunk
	}
	if wavHeader.NoteOk {
		riff.riffSize += 8 + note.noteSize
	}

	return wavHeader
}

// --- 头部加载与解析 (Header Loading & Parsing) ---

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

// readData is a helper to read structured data from io.Reader using binary.Read.
func readData(r io.Reader, data interface{}) error {
	return binary.Read(r, binary.BigEndian, data)
}

// loadHeader 从 io.ReadSeeker 中读取 HCA 头部信息.
func (h *Hca) loadHeader(r io.ReadSeeker) error {
	var sig uint32

	// HCA
	if err := readData(r, &sig); err != nil {
		return err
	}
	if sig&sigMask != sigHCA {
		return fmt.Errorf("invalid HCA signature: got %x", sig)
	}
	if err := h.hcaHeaderRead(r); err != nil {
		return err
	}

	// Read next signature
	if err := readData(r, &sig); err != nil {
		return err
	}

	// fmt
	if sig&sigMask != sigFMT {
		return fmt.Errorf("expected fmt signature, got %x", sig)
	}
	if err := h.fmtHeaderRead(r); err != nil {
		return err
	}
	if err := readData(r, &sig); err != nil {
		return err
	}

	// comp or dec
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

	// Optional chunks
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

	// Finalize initialization
	if !h.ath.Init(int(h.athType), h.samplingRate) {
		return fmt.Errorf("ATH init failed")
	}
	h.cipher = NewCipher()
	if !h.cipher.Init(int(h.ciphType), h.CiphKey1, h.CiphKey2) {
		return fmt.Errorf("cipher init failed")
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
	return nil
}

func ceil2(a, b uint32) uint32 {
	if b == 0 { return 0 }
	t := a / b
	if a%b != 0 { t++ }
	return t
}
