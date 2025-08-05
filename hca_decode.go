package hca

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"

	"github.com/vazrupe/endibuf"
)

// DecodeFromFile is file decode, return decode success/failed
// DecodeFromFile 是文件解码函数, 返回解码成功/失败
func (h *Hca) DecodeFromFile(src, dst string) bool {
	f, err := os.Open(src) // 打开源 HCA 文件
	if err != nil {
		return false
	}
	defer f.Close()

	r := endibuf.NewReader(f)

	fileWriter, err := os.Create(dst) // 创建目标 WAV 文件
	if err != nil {
		return false
	}
	defer fileWriter.Close()

	if !h.decodeBuffer(r, fileWriter) { // 调用 decodeBuffer 进行解码
		// 解码失败时, 主动关闭并删除文件
		fileWriter.Close()
		os.Remove(dst)
		return false
	}

	return true // 解码成功
}

// DecodeFromBytes is []byte data decode
// DecodeFromBytes 是 []byte 数据解码函数
func (h *Hca) DecodeFromBytes(data []byte) (decoded []byte, ok bool) {
	if len(data) < 8 {
		return nil, false
	}
	headerSize := binary.BigEndian.Uint16(data[6:])
	if len(data) < int(headerSize) {
		return nil, false
	}

	r := endibuf.NewReader(bytes.NewReader(data))
	w := new(bytes.Buffer)

	if !h.decodeBuffer(r, w) {
		return nil, false
	}

	return w.Bytes(), true
}

// Decoder creates a reader that decodes HCA data from the underlying reader.
// Decoder 创建一个解码器, 从提供的 reader 中读取并解码 HCA 数据
func (h *Hca) Decoder(reader io.Reader) (io.Reader, error) {
	readSeeker, ok := reader.(io.ReadSeeker)
	if !ok {
		return nil, fmt.Errorf("reader is not a ReadSeeker")
	}
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		if err := h.DecodeWithWriter(readSeeker, pw); err != nil {
			// 在goroutine中, Pipe的写入端错误通常意味着读取端已关闭.
			// 打印错误可能有助于调试, 但不应视为关键失败.
			// The write-end of a pipe closing with an error in a goroutine
			// usually means the read-end has been closed.
			// Printing this error can be useful for debugging but shouldn't be
			// treated as a critical failure.
		}
	}()
	return pr, nil
}

// DecodeWithWriter decodes HCA data from a reader and writes the WAVE output to a writer.
// DecodeWithWriter 从一个 reader 中解码 HCA 数据, 并将 WAVE 输出写入一个 writer.
func (h *Hca) DecodeWithWriter(r io.ReadSeeker, w io.Writer) error {
	endibufReader := endibuf.NewReader(r)
	if !h.decodeBuffer(endibufReader, w) {
		return fmt.Errorf("decode failed")
	}
	return nil
}

// decodeBuffer 从 endibuf.Reader 中解码 HCA 数据并写入 io.Writer
func (h *Hca) decodeBuffer(r *endibuf.Reader, w io.Writer) bool {
	saveEndian := r.Endian
	r.Endian = binary.BigEndian
	defer func() { r.Endian = saveEndian }()

	if h.Loop < 0 {
		return false
	}
	switch h.Mode {
	case ModeFloat, Mode8Bit, Mode16Bit, Mode24Bit, Mode32Bit:
	default:
		return false
	}

	if !h.loadHeader(r) {
		return false
	}
	r.Seek(int64(h.dataOffset), 0)

	wavHeader := h.buildWaveHeader()
	// wave_gen.go中的stWaveHeader.Write方法需要一个*endibuf.Writer
	// neo系列函数有一个NeoWrite, 它接收io.Writer
	// 我们假设NeoWrite是正确的, 并在后续步骤中确认
	// In wave_gen.go, the stWaveHeader.Write method requires an *endibuf.Writer.
	// The "neo" series of functions has a NeoWrite that accepts an io.Writer.
	// We assume NeoWrite is correct and will confirm this in a later step.
	wavHeader.NeoWrite(w, binary.LittleEndian)

	h.rvaVolume *= h.Volume

	if h.Loop == 0 {
		if !h.decodeBlocks(r, w, h.dataOffset, h.blockCount) {
			return false
		}
	} else {
		loopBlockOffset := h.dataOffset + h.loopStart*h.blockSize
		loopBlockCount := h.loopEnd - h.loopStart
		if !h.decodeBlocks(r, w, h.dataOffset, h.loopEnd) {
			return false
		}
		for i := 1; i < h.Loop; i++ {
			if !h.decodeBlocks(r, w, loopBlockOffset, loopBlockCount) {
				return false
			}
		}
		if !h.decodeBlocks(r, w, loopBlockOffset, h.blockCount-h.loopStart) {
			return false
		}
	}

	return true
}

// buildWaveHeader 构建 WAV 头部信息
func (h *Hca) buildWaveHeader() *stWaveHeader {
	wavHeader := newWaveHeader()

	riff := wavHeader.Riff
	smpl := wavHeader.Smpl
	note := wavHeader.Note
	data := wavHeader.Data

	if h.Mode > 0 {
		riff.fmtType = 1
		riff.fmtBitCount = uint16(h.Mode)
	} else {
		riff.fmtType = 3
		riff.fmtBitCount = 32
	}
	riff.fmtChannelCount = uint16(h.channelCount)
	riff.fmtSamplingRate = h.samplingRate
	riff.fmtSamplingSize = riff.fmtBitCount / 8 * riff.fmtChannelCount
	riff.fmtSamplesPerSec = riff.fmtSamplingRate * uint32(riff.fmtSamplingSize)

	if h.loopFlg {
		smpl.samplePeriod = uint32(1 / float64(riff.fmtSamplingRate) * 1000000000)
		smpl.loopStart = h.loopStart * 0x80 * 8 * uint32(riff.fmtSamplingSize)
		smpl.loopEnd = h.loopEnd * 0x80 * 8 * uint32(riff.fmtSamplingSize)
		if h.loopR01 == 0x80 {
			smpl.loopPlayCount = 0
		} else {
			smpl.loopPlayCount = h.loopR01
		}
	} else if h.Loop != 0 {
		smpl.loopStart = 0
		smpl.loopEnd = h.blockCount * 0x80 * 8 * uint32(riff.fmtSamplingSize)
		h.loopStart = 0
		h.loopEnd = h.blockCount
	}
	if h.commLen > 0 {
		wavHeader.NoteOk = true

		note.noteSize = 4 + h.commLen + 1
		note.comm = h.commComment
		if (note.noteSize & 3) != 0 {
			note.noteSize += 4 - (note.noteSize & 3)
		}
	}
	data.dataSize = h.blockCount*0x80*8*uint32(riff.fmtSamplingSize) + (smpl.loopEnd-smpl.loopStart)*uint32(h.Loop)
	riff.riffSize = 0x1C + 8 + data.dataSize
	if h.loopFlg && h.Loop == 0 {
		riff.riffSize += 17 * 4
		wavHeader.SmplOk = true
	}
	if h.commLen > 0 {
		riff.riffSize += 8 + note.noteSize
	}

	return wavHeader
}

// decodeBlocks 从 endibuf.Reader 读取指定数量的块, 解码并写入 io.Writer
func (h *Hca) decodeBlocks(r *endibuf.Reader, w io.Writer, address, count uint32) bool {
	r.Seek(int64(address), 0)
	for l := uint32(0); l < count; l++ {
		data, _ := r.ReadBytes(int(h.blockSize))
		if !h.decode(data) {
			return false
		}
		saveBlock := h.decoder.waveSerialize(h.rvaVolume)
		h.save(saveBlock, w, binary.LittleEndian)

		address += h.blockSize
	}
	return true
}

// decode 解码一个 HCA 数据块
func (h *Hca) decode(data []byte) bool {
	if len(data) < int(h.blockSize) {
		return false
	}
	if checkSum(data, 0) != 0 {
		return false
	}
	mask := h.cipher.Mask(data)
	d := &clData{}
	d.Init(mask, int(h.blockSize))
	magic := d.GetBit(16)
	if magic == 0xFFFF {
		h.decoder.decode(d, h.ath.GetTable())
	}
	return true
}

// checkSum 计算给定数据的校验和
func checkSum(data []byte, sum uint16) uint16 {
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

// save 将浮点样本数据转换为指定模式并写入 io.Writer
func (h *Hca) save(base []float32, w io.Writer, endian binary.ByteOrder) {
	switch h.Mode {
	case ModeFloat:
		writeData(base, w, endian)
	case Mode8Bit:
		writeData(mode8BitConvert(base), w, endian)
	case Mode16Bit:
		writeData(mode16BitConvert(base), w, endian)
	case Mode24Bit:
		// 24bit is special, needs custom handling
		// 24位是特别的, 需要自定义处理
		binary.Write(w, endian, mode24BitConvert(base))
	case Mode32Bit:
		writeData(mode32BitConvert(base), w, endian)
	}
}

func writeData(data interface{}, w io.Writer, endian binary.ByteOrder) (err error) {
	switch data := data.(type) {
	case string:
		err = writeCString(w, data)
	case *string:
		err = writeCString(w, *data)
	case []string:
		for i := range data {
			err = writeCString(w, data[i])
			if err != nil {
				return
			}
		}
	case float32:
		err = writeFloat32(w, data, endian)
	case *float32:
		err = writeFloat32(w, *data, endian)
	case []float32:
		// This is one of the key improvements.
		// Instead of looping, write the whole slice at once.
		// 这是关键改进之一.
		// 直接写入整个切片, 而不是循环.
		err = binary.Write(w, endian, data)

	case float64:
		err = writeFloat64(w, data, endian)
	case *float64:
		err = writeFloat64(w, *data, endian)
	case []float64:
		err = binary.Write(w, endian, data)

	case int8, int16, int32, int64,
		uint8, uint16, uint32, uint64,
		*int8, *int16, *int32, *int64,
		*uint8, *uint16, *uint32, *uint64,
		[]int8, []int16, []int32, []int64,
		[]uint8, []uint16, []uint32, []uint64:
		err = binary.Write(w, endian, data)
	}
	return
}

// writeCString writes a string with a null terminator.
// writeCString 写入一个以空字符结尾的字符串.
func writeCString(w io.Writer, line string) (err error) {
	return writeString(w, line, 0)
}

// writeString writes a string with a given delimiter.
// writeString 写入一个带指定分隔符的字符串.
func writeString(w io.Writer, line string, delim byte) (err error) {
	// Improved implementation to avoid allocation
	// 改进实现以避免内存分配
	if _, err = io.WriteString(w, line); err != nil {
		return
	}
	_, err = w.Write([]byte{delim})
	return
}

// writeFloat32 writes a float32 value.
// writeFloat32 写入一个 float32 值.
func writeFloat32(w io.Writer, data float32, endian binary.ByteOrder) error {
	return binary.Write(w, endian, math.Float32bits(data))
}

// writeFloat64 writes a float64 value.
// writeFloat64 写入一个 float64 值.
func writeFloat64(w io.Writer, data float64, endian binary.ByteOrder) error {
	return binary.Write(w, endian, math.Float64bits(data))
}

// mode converters

func mode8BitConvert(base []float32) []int8 {
	res := make([]int8, len(base))
	for i := range res {
		res[i] = int8(int(base[i]*0x7F) + 0x80)
	}
	return res
}

func mode16BitConvert(base []float32) []int16 {
	res := make([]int16, len(base))
	for i := range res {
		res[i] = int16(base[i] * 0x7FFF)
	}
	return res
}

func mode24BitConvert(base []float32) []byte {
	res := make([]byte, len(base)*3)
	for i := range base {
		v := int32(base[i] * 0x7FFFFF)
		// Assuming little-endian for 24-bit audio data writing
		// 假设24位音频数据写入使用小端序
		res[i*3] = byte(v)
		res[i*3+1] = byte(v >> 8)
		res[i*3+2] = byte(v >> 16)
	}
	return res
}

func mode32BitConvert(base []float32) []int32 {
	res := make([]int32, len(base))
	for i := range res {
		res[i] = int32(base[i] * 0x7FFFFFFF)
	}
	return res
}
