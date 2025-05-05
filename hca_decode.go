package hca

import (
	"bytes"           // 导入 bytes 包，用于处理字节切片
	"encoding/binary" // 导入 encoding/binary 包，用于处理字节序
	"io"              // 导入 io 包，用于输入输出操作
	"os"              // 导入 os 包，用于操作系统相关操作

	"github.com/vazrupe/endibuf" // 导入 endibuf 库
)

// DecodeFromFile is file decode, return decode success/failed
// DecodeFromFile 是文件解码函数，返回解码成功/失败
func (h *Hca) DecodeFromFile(src, dst string) bool {
	f, err := os.Open(src) // 打开源 HCA 文件
	if err != nil {        // 如果打开文件失败
		return false // 返回 false
	}
	defer f.Close()           // 确保文件关闭
	r := endibuf.NewReader(f) // 创建一个 endibuf.Reader 来读取文件
	f2, err := os.Create(dst) // 创建目标 WAV 文件
	if err != nil {           // 如果创建文件失败
		return false // 返回 false
	}
	w := endibuf.NewWriter(f2) // 创建一个 endibuf.Writer 来写入文件

	success := h.decodeBuffer(r, w) // 调用 decodeBuffer 进行解码

	f2.Close()    // 关闭目标文件
	if !success { // 如果解码失败
		os.Remove(dst) // 删除不完整或错误的输出文件
		return false   // 返回 false
	}

	return true // 解码成功返回 true
}

// DecodeFromBytes is []byte data decode
// DecodeFromBytes 是 []byte 数据解码函数
func (h *Hca) DecodeFromBytes(data []byte) (decoded []byte, ok bool) {
	decodedData := []byte{} // 初始化解码后的数据切片

	if len(data) < 8 { // 检查数据长度是否足够包含基本头部信息
		return decodedData, false // 长度不足返回 false
	}

	headerSize := binary.BigEndian.Uint16(data[6:]) // 从头部信息中读取头部大小
	if len(data) < int(headerSize) {                // 检查数据长度是否足够包含完整的头部
		return decodedData, false // 长度不足返回 false
	}

	// create read buffer
	// 创建读取缓冲区
	base := bytes.NewReader(data)                    // 创建一个 bytes.Reader 来从字节切片读取
	buf := io.NewSectionReader(base, 0, base.Size()) // 创建一个 io.SectionReader，以便像文件一样读取
	r := endibuf.NewReader(buf)                      // 创建一个 endibuf.Reader

	// create temp file (write)
	// 创建临时文件（用于写入）
	tempfile, _ := os.CreateTemp("", "hca_wav_temp_") // 创建一个临时文件
	defer os.Remove(tempfile.Name())                  // 确保临时文件被删除
	w := endibuf.NewWriter(tempfile)                  // 创建一个 endibuf.Writer
	w.Endian = binary.LittleEndian                    // 设置写入字节序为小端序

	if !h.decodeBuffer(r, w) { // 调用 decodeBuffer 进行解码
		return decodedData, false // 解码失败返回 false
	}

	tempfile.Seek(0, 0)                   // 将临时文件指针移到开头
	decodedData, _ = io.ReadAll(tempfile) // 读取临时文件的所有内容

	return decodedData, true // 返回解码后的数据和成功标志
}

// decodeBuffer 从 endibuf.Reader 中解码 HCA 数据并写入 endibuf.Writer
func (h *Hca) decodeBuffer(r *endibuf.Reader, w *endibuf.Writer) bool {
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
	w.Endian = binary.LittleEndian // 设置写入字节序为小端序

	wavHeader := h.buildWaveHeader() // 构建 WAV 头部信息
	wavHeader.Write(w)               // 将 WAV 头部写入 Writer

	// adjust the relative volume
	// 调整相对音量
	h.rvaVolume *= h.Volume // 将 RVA 音量与用户指定的音量相乘

	// decode
	// 解码
	if h.Loop == 0 { // 如果没有设置循环次数
		if !h.decodeFromBytesDecode(r, w, h.dataOffset, h.blockCount) { // 解码从数据开始到总块数
			return false // 解码失败返回 false
		}
	} else { // 如果设置了循环次数
		loopBlockOffset := h.dataOffset + h.loopStart*h.blockSize    // 计算循环开始块的偏移量
		loopBlockCount := h.loopEnd - h.loopStart                    // 计算循环块的数量
		if !h.decodeFromBytesDecode(r, w, h.dataOffset, h.loopEnd) { // 解码从数据开始到循环结束块
			return false // 解码失败返回 false
		}
		for i := 1; i < h.Loop; i++ { // 循环指定次数
			if !h.decodeFromBytesDecode(r, w, loopBlockOffset, loopBlockCount) { // 解码循环部分的块
				return false // 解码失败返回 false
			}
		}
		if !h.decodeFromBytesDecode(r, w, loopBlockOffset, h.blockCount-h.loopStart) { // 解码从循环开始块到总块数（这部分处理剩余的尾部数据）
			return false // 解码失败返回 false
		}
	}

	r.Endian = saveEndian // 恢复原始的读取字节序设置

	return true // 解码成功返回 true
}

// buildWaveHeader 构建 WAV 头部信息
func (h *Hca) buildWaveHeader() *stWaveHeader {
	wavHeader := newWaveHeader() // 创建新的 WAV 头部结构体

	riff := wavHeader.Riff // 获取 Riff 块
	smpl := wavHeader.Smpl // 获取 Smpl 块
	note := wavHeader.Note // 获取 Note 块
	data := wavHeader.Data // 获取 Data 块

	if h.Mode > 0 { // 如果模式大于 0 (非浮点模式)
		riff.fmtType = 1                  // 设置 fmt 类型为 1 (PCM)
		riff.fmtBitCount = uint16(h.Mode) // 设置每样本位数
	} else { // 如果是浮点模式
		riff.fmtType = 3      // 设置 fmt 类型为 3 (IEEE Float)
		riff.fmtBitCount = 32 // 设置每样本位数为 32
	}
	riff.fmtChannelCount = uint16(h.channelCount)                               // 设置通道数量
	riff.fmtSamplingRate = h.samplingRate                                       // 设置采样率
	riff.fmtSamplingSize = riff.fmtBitCount / 8 * riff.fmtChannelCount          // 计算每样本字节数
	riff.fmtSamplesPerSec = riff.fmtSamplingRate * uint32(riff.fmtSamplingSize) // 计算每秒字节数

	if h.loopFlg { // 如果有循环标志
		smpl.samplePeriod = uint32(1 / float64(riff.fmtSamplingRate) * 1000000000) // 计算样本周期
		smpl.loopStart = h.loopStart * 0x80 * 8 * uint32(riff.fmtSamplingSize)     // 计算循环开始的字节偏移量
		smpl.loopEnd = h.loopEnd * 0x80 * 8 * uint32(riff.fmtSamplingSize)         // 计算循环结束的字节偏移量
		if h.loopR01 == 0x80 {                                                     // 如果 loopR01 是 0x80 (无限循环)
			smpl.loopPlayCount = 0 // 设置循环播放次数为 0 (无限)
		} else {
			smpl.loopPlayCount = h.loopR01 // 否则设置循环播放次数
		}
	} else if h.Loop != 0 { // 如果没有循环标志但用户指定了循环次数
		smpl.loopStart = 0                                                    // 设置循环开始为 0
		smpl.loopEnd = h.blockCount * 0x80 * 8 * uint32(riff.fmtSamplingSize) // 设置循环结束为总样本数的字节偏移量
		h.loopStart = 0                                                       // 将 HCA 结构体中的循环开始和结束更新为总范围
		h.loopEnd = h.blockCount
	}
	if h.commLen > 0 { // 如果有注释
		wavHeader.NoteOk = true // 标记 Note 块存在

		note.noteSize = 4 + h.commLen + 1 // 计算 Note 块的大小 (4字节长度 + 注释长度 + 1字节结束符)
		note.comm = h.commComment         // 设置注释内容
		if (note.noteSize & 3) != 0 {     // 如果 Note 块大小不是 4 的倍数
			note.noteSize += 4 - (note.noteSize & 3) // 填充到 4 的倍数
		}
	}
	data.dataSize = h.blockCount*0x80*8*uint32(riff.fmtSamplingSize) + (smpl.loopEnd-smpl.loopStart)*uint32(h.Loop) // 计算数据块大小 (总样本数 + 循环部分的样本数 * (循环次数-1))
	riff.riffSize = 0x1C + 8 + data.dataSize                                                                        // 计算 Riff 块大小 (固定部分 + 数据块大小)
	if h.loopFlg && h.Loop == 0 {                                                                                   // 如果有循环标志且用户没有指定循环次数 (使用 HCA 原生的循环)
		// smpl Size
		riff.riffSize += 17 * 4 // 添加 Smpl 块的大小
		wavHeader.SmplOk = true // 标记 Smpl 块存在
	}
	if h.commLen > 0 { // 如果有注释
		riff.riffSize += 8 + note.noteSize // 添加 Note 块的大小
	}

	return wavHeader // 返回构建好的 WAV 头部结构体
}

// decodeFromBytesDecode 从 endibuf.Reader 读取指定数量的块，解码并写入 endibuf.Writer
func (h *Hca) decodeFromBytesDecode(r *endibuf.Reader, w *endibuf.Writer, address, count uint32) bool {
	r.Seek(int64(address), 0)            // 将读取位置移动到指定的地址
	for l := uint32(0); l < count; l++ { // 循环指定数量的块
		data, _ := r.ReadBytes(int(h.blockSize)) // 读取一个块的数据
		if !h.decode(data) {                     // 解码当前块
			return false // 解码失败返回 false
		}
		saveBlock := h.decoder.waveSerialize(h.rvaVolume) // 将解码后的波形数据序列化
		h.save(saveBlock, w)                              // 保存波形数据到 Writer

		address += h.blockSize // 更新地址到下一个块的开始处
	}
	return true // 所有块解码成功返回 true
}

// decode 解码一个 HCA 数据块
func (h *Hca) decode(data []byte) bool {
	// block data
	// 块数据
	if len(data) < int(h.blockSize) { // 检查数据长度是否与块大小匹配
		return false // 不匹配返回 false
	}
	if checkSum(data, 0) != 0 { // 检查校验和
		return false // 校验和错误返回 false
	}
	mask := h.cipher.Mask(data)    // 使用密码对数据进行掩码操作（解密）
	d := &clData{}                 // 创建 clData 对象（假设 clData 是一个比特读取器结构体）
	d.Init(mask, int(h.blockSize)) // 初始化 clData，使用解密后的数据
	magic := d.GetBit(16)          // 读取块的魔术数字 (通常应该是 0xFFFF)
	if magic == 0xFFFF {           // 如果魔术数字正确
		h.decoder.decode(d, h.ath.GetTable()) // 调用通道解码器进行解码
	}
	return true // 解码成功返回 true (即使 magic 不为 0xFFFF，只要 checkSum 通过也返回 true，这可能需要根据实际 HCA 规范确认行为)
}

// checkSum 计算给定数据的校验和
func checkSum(data []byte, sum uint16) uint16 {
	res := sum     // 初始化校验和结果
	v := []uint16{ // 校验和查找表
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
	for i := 0; i < len(data); i++ { // 遍历数据字节
		res = (res << 8) ^ v[byte(res>>8)^data[i]] // 计算校验和
	}
	return res // 返回计算出的校验和
}

// save 将浮点样本数据转换为指定模式并写入 endibuf.Writer
func (h *Hca) save(base []float32, w *endibuf.Writer) {
	switch h.Mode { // 根据指定的模式进行转换和写入
	case ModeFloat: // 浮点模式
		w.WriteData(base) // 直接写入浮点数据
	case Mode8Bit: // 8 位模式
		w.WriteData(mode8BitConvert(base)) // 转换为 8 位整型并写入
	case Mode16Bit: // 16 位模式
		w.WriteData(mode16BitConvert(base)) // 转换为 16 位整型并写入
	case Mode24Bit: // 24 位模式
		w.WriteData(mode24BitConvert(base)) // 转换为 24 位字节切片并写入
	case Mode32Bit: // 32 位模式
		w.WriteData(mode32BitConvert(base)) // 转换为 32 位整型并写入
	}
}

// mode8BitConvert 将 float32 切片转换为 8 位整型切片
func mode8BitConvert(base []float32) []int8 {
	res := make([]int8, len(base)) // 创建新的 int8 切片
	for i := range res {           // 遍历浮点切片
		res[i] = int8(int(base[i]*0x7F) + 0x80) // 转换为 8 位整型，并偏移 0x80 (使其范围为 0 到 255)
	}
	return res // 返回转换后的切片
}

// mode16BitConvert 将 float32 切片转换为 16 位整型切片
func mode16BitConvert(base []float32) []int16 {
	res := make([]int16, len(base)) // 创建新的 int16 切片
	for i := range res {            // 遍历浮点切片
		res[i] = int16(base[i] * 0x7FFF) // 转换为 16 位整型
	}
	return res // 返回转换后的切片
}

// mode24BitConvert 将 float32 切片转换为 24 位字节切片
func mode24BitConvert(base []float32) []byte {
	res := make([]byte, len(base)*3) // 创建新的字节切片，大小为 float32 切片长度的 3 倍

	for i := range base { // 遍历浮点切片
		v := int32(base[i] * 0x7FFFFF) // 转换为 24 位有符号整数 (0x7FFFFF 是 2^23 - 1)
		// 将 24 位整数拆分为 3 个字节（大端序）
		res[i*3] = byte((v & 0xFF0000) >> 16)
		res[i*3+1] = byte((v & 0xFF00) >> 8)
		res[i*3+2] = byte((v & 0xFF))
	}
	return res // 返回转换后的字节切片
}

// mode32BitConvert 将 float32 切片转换为 32 位整型切片
func mode32BitConvert(base []float32) []int32 {
	res := make([]int32, len(base)) // 创建新的 int32 切片
	for i := range res {            // 遍历浮点切片
		res[i] = int32(base[i] * 0x7FFFFFFF) // 转换为 32 位整型
	}
	return res // 返回转换后的切片
}
