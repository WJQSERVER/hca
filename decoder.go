package hca

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"

	"github.com/vazrupe/endibuf"
)

// DecodeFromFile is file decode, return decode success/failed
// DecodeFromFile 是文件解码函数，返回解码成功/失败
func (h *Hca) NeoDecodeFromFile(src, dst string) bool {
	f, err := os.Open(src) // 打开源 HCA 文件
	if err != nil {        // 如果打开文件失败
		return false // 返回 false
	}
	defer f.Close()                   // 确保文件关闭
	r := endibuf.NewReader(f)         // 创建一个 endibuf.Reader 来读取文件
	fileWriter, err := os.Create(dst) // 创建目标 WAV 文件
	if err != nil {                   // 如果创建文件失败
		return false // 返回 false
	}

	success := h.neoDecodeBuffer(r, fileWriter) // 调用 decodeBuffer 进行解码

	fileWriter.Close() // 关闭目标文件
	if !success {      // 如果解码失败
		os.Remove(dst) // 删除不完整或错误的输出文件
		return false   // 返回 false
	}

	return true // 解码成功返回 true
}

func (h *Hca) Decoder(reader io.Reader) (io.Reader, error) {
	// 调用DecodeWithWriter, 并使用pipe连接
	reader, ok := reader.(io.ReadSeeker)
	if !ok {
		return nil, fmt.Errorf("reader is not a ReadSeeker")
	}
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		if err := h.DecodeWithWriter(reader.(io.ReadSeeker), pw); err != nil {
			fmt.Printf("decode error: %v\n", err)
		}
	}()
	return pr, nil
}

func (h *Hca) DecodeWithWriter(r io.ReadSeeker, w io.Writer) error {
	endibufReader := endibuf.NewReader(r)
	//endibufWriter := endibuf.NewWriter(w)
	//success := h.decodeBuffer(endibufReader, endibufWriter)
	success := h.neoDecodeBuffer(endibufReader, w)
	if !success {
		return fmt.Errorf("decode failed")
	}

	return nil // 解码成功返回 nil 错误
}

// decodeBuffer 从 endibuf.Reader 中解码 HCA 数据并写入 endibuf.Writer
func (h *Hca) neoDecodeBuffer(r *endibuf.Reader, w io.Writer) bool {
	saveEndian := r.Endian // 保存当前的读取字节序设置

	r.Endian = binary.BigEndian // 将读取字节序设置为大端序

	// size check
	// 大小检查
	if h.Loop < 0 { // 检查循环次数是否有效
		return false // 无效返回 false
	}
	switch h.Mode { // 检查写入模式是否有效
	case ModeFloat, Mode8Bit, Mode16Bit, Mode24Bit, Mode32Bit:
		break // 有效模式，继续
	default:
		return false // 无效模式返回 false
	}

	// header read
	// 读取头部
	if !h.loadHeader(r) { // 读取 HCA 头部信息
		return false // 读取失败返回 false
	}
	r.Seek(int64(h.dataOffset), 0) // 将读取位置移动到数据开始处

	// create temp file (write)
	// 创建临时文件（用于写入，此行注释可能重复或指代 W 的初始化）

	wavHeader := h.buildWaveHeader()           // 构建 WAV 头部信息
	wavHeader.NeoWrite(w, binary.LittleEndian) // 将 WAV 头部写入 Writer

	// adjust the relative volume
	// 调整相对音量
	h.rvaVolume *= h.Volume // 将 RVA 音量与用户指定的音量相乘

	// decode
	// 解码
	if h.Loop == 0 { // 如果没有设置循环次数
		if !h.neoDecodeFromBytesDecode(r, w, h.dataOffset, h.blockCount) { // 解码从数据开始到总块数
			return false // 解码失败返回 false
		}
	} else { // 如果设置了循环次数
		loopBlockOffset := h.dataOffset + h.loopStart*h.blockSize       // 计算循环开始块的偏移量
		loopBlockCount := h.loopEnd - h.loopStart                       // 计算循环块的数量
		if !h.neoDecodeFromBytesDecode(r, w, h.dataOffset, h.loopEnd) { // 解码从数据开始到循环结束块
			return false // 解码失败返回 false
		}
		for i := 1; i < h.Loop; i++ { // 循环指定次数
			if !h.neoDecodeFromBytesDecode(r, w, loopBlockOffset, loopBlockCount) { // 解码循环部分的块
				return false // 解码失败返回 false
			}
		}
		if !h.neoDecodeFromBytesDecode(r, w, loopBlockOffset, h.blockCount-h.loopStart) { // 解码从循环开始块到总块数（这部分处理剩余的尾部数据）
			return false // 解码失败返回 false
		}
	}

	r.Endian = saveEndian // 恢复原始的读取字节序设置

	return true // 解码成功返回 true
}

// decodeFromBytesDecode 从 endibuf.Reader 读取指定数量的块，解码并写入 endibuf.Writer
func (h *Hca) neoDecodeFromBytesDecode(r *endibuf.Reader, w io.Writer, address, count uint32) bool {
	r.Seek(int64(address), 0)            // 将读取位置移动到指定的地址
	for l := uint32(0); l < count; l++ { // 循环指定数量的块
		data, _ := r.ReadBytes(int(h.blockSize)) // 读取一个块的数据
		if !h.decode(data) {                     // 解码当前块
			return false // 解码失败返回 false
		}
		saveBlock := h.decoder.waveSerialize(h.rvaVolume) // 将解码后的波形数据序列化
		h.neoSave(saveBlock, w, binary.LittleEndian)      // 保存波形数据到 Writer

		address += h.blockSize // 更新地址到下一个块的开始处
	}
	return true // 所有块解码成功返回 true
}

// save 将浮点样本数据转换为指定模式并写入 endibuf.Writer
func (h *Hca) neoSave(base []float32, w io.Writer, endian binary.ByteOrder) {
	switch h.Mode { // 根据指定的模式进行转换和写入
	case ModeFloat: // 浮点模式
		WriteData(base, w, endian) // 直接写入浮点数据
	case Mode8Bit: // 8 位模式
		WriteData(mode8BitConvert(base), w, endian) // 转换为 8 位整型并写入
	case Mode16Bit: // 16 位模式
		WriteData(mode16BitConvert(base), w, endian) // 转换为 16 位整型并写入
	case Mode24Bit: // 24 位模式
		WriteData(mode24BitConvert(base), w, endian) // 转换为 24 位字节切片并写入

	case Mode32Bit: // 32 位模式
		WriteData(mode32BitConvert(base), w, endian) // 转换为 32 位整型并写入
	}
}

func WriteData(data interface{}, w io.Writer, endian binary.ByteOrder) (err error) {
	switch data := data.(type) {
	case string:
		err = WriteCString(w, data)
	case *string:
		err = WriteCString(w, *data)
	case []string:
		for i := range data {
			err = WriteCString(w, data[i])
			if err != nil {
				return
			}
		}
	case float32:
		err = WriteFloat32(w, data, endian)
	case *float32:
		err = WriteFloat32(w, *data, endian)
	case []float32:
		for i := range data {
			err = WriteFloat32(w, data[i], endian)
			if err != nil {
				return
			}
		}
	case float64:
		err = WriteFloat64(w, data, endian)
	case *float64:
		err = WriteFloat64(w, *data, endian)
	case []float64:
		for i := range data {
			err = WriteFloat64(w, data[i], endian)
			if err != nil {
				return
			}
		}
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

// WriteCString return string (append zero byte)
func WriteCString(w io.Writer, line string) (err error) {
	return WriteString(w, line, byte(0))
}

// WriteString return string (append zero byte)
func WriteString(w io.Writer, line string, delim byte) (err error) {
	line += string(delim)
	b := []byte(line)
	_, err = w.Write(b)
	return
}

// WriteFloat32 return float32
func WriteFloat32(w io.Writer, data float32, endian binary.ByteOrder) error {
	tmp := math.Float32bits(data)
	return binary.Write(w, endian, tmp)
}

// WriteFloat64 return float64
func WriteFloat64(w io.Writer, data float64, endian binary.ByteOrder) error {
	tmp := math.Float64bits(data)
	return binary.Write(w, endian, tmp)
}
