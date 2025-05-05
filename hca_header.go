package hca

import (
	"encoding/binary" // 导入 encoding/binary 包，用于处理字节序

	"github.com/vazrupe/endibuf" // 导入 endibuf 库
)

const (
	sigMask = 0x7F7F7F7F // 签名掩码
	sigHCA  = 0x48434100 // HCA 签名
	sigFMT  = 0x666D7400 // fmt 签名
	sigCOMP = 0x636F6D70 // comp 签名
	sigDEC  = 0x64656300 // dec 签名
	sigVBR  = 0x76627200 // vbr 签名
	sigATH  = 0x61746800 // ath 签名
	sigLOOP = 0x6C6F6F70 // loop 签名
	sigCIPH = 0x63697068 // ciph 签名
	sigRVA  = 0x72766100 // rva 签名
	sigCOMM = 0x636F6D6D // comm 签名
)

// loadHeader 从 endibuf.Reader 中读取 HCA 头部信息
func (h *Hca) loadHeader(r *endibuf.Reader) bool {
	endianSave := r.Endian      // 保存当前的字节序设置
	r.Endian = binary.BigEndian // 将字节序设置为大端序

	var sig uint32 // 用于存储读取的块签名

	// HCA 块
	r.ReadData(&sig)           // 读取 HCA 块签名
	if sig&sigMask == sigHCA { // 检查签名是否匹配 HCA
		if !h.hcaHeaderRead(r) { // 读取 HCA 头部详细信息
			return false // 读取失败返回 false
		}

		r.ReadData(&sig) // 读取下一个块签名
	} else {
		return false // HCA 签名不匹配返回 false
	}

	// fmt 块
	if sig&sigMask == sigFMT { // 检查签名是否匹配 fmt
		if !h.fmtHeaderRead(r) { // 读取 fmt 头部详细信息
			return false // 读取失败返回 false
		}
		r.ReadData(&sig) // 读取下一个块签名
	} else {
		return false // fmt 签名不匹配返回 false
	}

	if sig&sigMask == sigCOMP { // 检查签名是否匹配 comp
		// comp 块
		if !h.compHeaderRead(r) { // 读取 comp 头部详细信息
			return false // 读取失败返回 false
		}
		r.ReadData(&sig) // 读取下一个块签名
	} else if sig&sigMask == sigDEC { // 检查签名是否匹配 dec
		// dec 块
		if !h.decHeaderRead(r) { // 读取 dec 头部详细信息
			return false // 读取失败返回 false
		}
		r.ReadData(&sig) // 读取下一个块签名
	} else {
		return false // comp 或 dec 签名不匹配返回 false
	}

	// vbr 块
	if sig&sigMask == sigVBR { // 检查签名是否匹配 vbr
		if !h.vbrHeaderRead(r) { // 读取 vbr 头部详细信息
			return false // 读取失败返回 false
		}
		r.ReadData(&sig) // 读取下一个块签名
	} else {
		h.vbrR01 = 0 // 如果没有 vbr 块，设置默认值
		h.vbrR02 = 0
	}

	// ath 块
	if sig&sigMask == sigATH { // 检查签名是否匹配 ath
		if !h.athHeaderRead(r) { // 读取 ath 头部详细信息
			return false // 读取失败返回 false
		}
		r.ReadData(&sig) // 读取下一个块签名
	} else {
		if h.version < 0x200 { // 如果没有 ath 块，根据版本设置默认类型
			h.athType = 1
		} else {
			h.athType = 0
		}
	}

	// loop 块
	if sig&sigMask == sigLOOP { // 检查签名是否匹配 loop
		if !h.loopHeaderRead(r) { // 读取 loop 头部详细信息
			return false // 读取失败返回 false
		}
		r.ReadData(&sig) // 读取下一个块签名
	} else {
		h.loopStart = 0 // 如果没有 loop 块，设置默认值
		h.loopEnd = 0
		h.loopR01 = 0
		h.loopR02 = 0x400
		h.loopFlg = false
	}

	// ciph 块
	if sig&sigMask == sigCIPH { // 检查签名是否匹配 ciph
		if !h.ciphHeaderRead(r) { // 读取 ciph 头部详细信息
			return false // 读取失败返回 false
		}
		r.ReadData(&sig) // 读取下一个块签名
	} else {
		h.ciphType = 0 // 如果没有 ciph 块，设置默认类型为 0 (无密码)
	}

	// rva 块
	if sig&sigMask == sigRVA { // 检查签名是否匹配 rva
		if !h.rvaHeaderRead(r) { // 读取 rva 头部详细信息
			return false // 读取失败返回 false
		}
		r.ReadData(&sig) // 读取下一个块签名
	} else {
		h.rvaVolume = 1 // 如果没有 rva 块，设置默认音量为 1
	}

	// comm 块
	if sig&sigMask == sigCOMM { // 检查签名是否匹配 comm
		if !h.commHeaderRead(r) { // 读取 comm 头部详细信息
			return false // 读取失败返回 false
		}
	} else {
		h.commLen = 0 // 如果没有 comm 块，设置默认值
		h.commComment = ""
	}

	// 初始化
	if !h.ath.Init(int(h.athType), h.samplingRate) { // 初始化 ATH
		return false // 初始化失败返回 false
	}
	h.cipher = NewCipher()                                       // 创建新的密码对象
	if !h.cipher.Init(int(h.ciphType), h.CiphKey1, h.CiphKey2) { // 初始化密码
		return false // 初始化失败返回 false
	}

	// 数值检查（为了避免头部修改错误引起的错误）
	if h.compR03 == 0 {
		h.compR03 = 1 // 如果 compR03 为 0，设置为 1
	}

	// 解码准备
	if !(h.compR01 == 1 && h.compR02 == 15) { // 检查 compR01 和 compR02 的特定值
		return false // 不匹配返回 false
	}
	h.compR09 = ceil2(h.compR05-(h.compR06+h.compR07), h.compR08)                                                              // 计算 compR09
	h.decoder = newChannelDecoder(h.channelCount, h.compR03, h.compR04, h.compR05, h.compR06, h.compR07, h.compR08, h.compR09) // 创建新的通道解码器

	r.Endian = endianSave // 恢复原始的字节序设置
	return true           // 头部读取成功返回 true
}

// hcaHeaderRead 读取 HCA 块的详细信息
func (h *Hca) hcaHeaderRead(r *endibuf.Reader) bool {
	version, _ := r.ReadUint16()    // 读取版本
	dataOffset, _ := r.ReadUint16() // 读取数据偏移量
	h.version = uint32(version)
	h.dataOffset = uint32(dataOffset)
	return true // 读取成功返回 true
}

// fmtHeaderRead 读取 fmt 块的详细信息
func (h *Hca) fmtHeaderRead(r *endibuf.Reader) bool {
	ui, _ := r.ReadUint32()                  // 读取一个 uint32 字段
	h.channelCount = (ui & 0xFF000000) >> 24 // 提取通道数量
	h.samplingRate = ui & 0x00FFFFFF         // 提取采样率
	h.blockCount, _ = r.ReadUint32()         // 读取块总数
	fmtR01, _ := r.ReadUint16()              // 读取 fmtR01
	fmtR02, _ := r.ReadUint16()              // 读取 fmtR02
	h.fmtR01 = uint32(fmtR01)
	h.fmtR02 = uint32(fmtR02)
	if !(h.channelCount >= 1 && h.channelCount <= 16) { // 检查通道数量的有效范围
		return false // 无效返回 false
	}
	if !(h.samplingRate >= 1 && h.samplingRate <= 0x7FFFFF) { // 检查采样率的有效范围
		return false // 无效返回 false
	}
	return true // 读取成功返回 true
}

// compHeaderRead 读取 comp 块的详细信息
func (h *Hca) compHeaderRead(r *endibuf.Reader) bool {
	blockSize, _ := r.ReadUint16() // 读取块大小
	h.blockSize = uint32(blockSize)
	datas, _ := r.ReadBytes(10) // 读取接下来的 10 个字节
	h.compR01 = uint32(datas[0])
	h.compR02 = uint32(datas[1])
	h.compR03 = uint32(datas[2])
	h.compR04 = uint32(datas[3])
	h.compR05 = uint32(datas[4])
	h.compR06 = uint32(datas[5])
	h.compR07 = uint32(datas[6])
	h.compR08 = uint32(datas[7])
	if !((h.blockSize >= 8 && h.blockSize <= 0xFFFF) || (h.blockSize == 0)) { // 检查块大小的有效范围
		return false // 无效返回 false
	}
	if !(h.compR01 >= 0 && h.compR01 <= h.compR02 && h.compR02 <= 0x1F) { // 检查 compR01 和 compR02 的有效范围
		return false // 无效返回 false
	}
	return true // 读取成功返回 true
}

// decHeaderRead 读取 dec 块的详细信息
func (h *Hca) decHeaderRead(r *endibuf.Reader) bool {
	blockSize, _ := r.ReadUint16() // 读取块大小
	h.blockSize = uint32(blockSize)
	datas, _ := r.ReadBytes(6) // 读取接下来的 6 个字节
	h.compR01 = uint32(datas[0])
	h.compR02 = uint32(datas[1])
	h.compR03 = uint32(datas[4] & 0xF) // 提取 compR03
	h.compR04 = uint32(datas[4] >> 4)  // 提取 compR04
	h.compR05 = uint32(datas[2]) + 1   // 计算 compR05
	if datas[5] > 0 {                  // 根据 datas[5] 计算 compR06
		h.compR06 = uint32(datas[3]) + 1
	} else {
		h.compR06 = uint32(datas[2]) + 1
	}
	h.compR07 = h.compR05 - h.compR06                                       // 计算 compR07
	h.compR08 = 0                                                           // compR08 在 dec 块中为 0
	if !((h.blockSize >= 8 && h.blockSize <= 0xFFFF) || h.blockSize == 0) { // 检查块大小的有效范围
		return false // 无效返回 false
	}
	if !(h.compR01 >= 0 && h.compR01 <= h.compR02 && h.compR02 <= 0x1F) { // 检查 compR01 和 compR02 的有效范围
		return false // 无效返回 false
	}
	if h.compR03 == 0 { // 如果 compR03 为 0，设置为 1
		h.compR03 = 1
	}
	return true // 读取成功返回 true
}

// vbrHeaderRead 读取 vbr 块的详细信息
func (h *Hca) vbrHeaderRead(r *endibuf.Reader) bool {
	tmp, _ := r.ReadUint16() // 读取 vbrR01
	h.vbrR01 = uint32(tmp)
	tmp, _ = r.ReadUint16() // 读取 vbrR02
	h.vbrR02 = uint32(tmp)
	return true // 读取成功返回 true
}

// athHeaderRead 读取 ath 块的详细信息
func (h *Hca) athHeaderRead(r *endibuf.Reader) bool {
	tmp, _ := r.ReadUint16() // 读取 athType
	h.athType = uint32(tmp)
	return true // 读取成功返回 true
}

// loopHeaderRead 读取 loop 块的详细信息
func (h *Hca) loopHeaderRead(r *endibuf.Reader) bool {
	h.loopStart, _ = r.ReadUint32() // 读取循环开始块索引
	h.loopEnd, _ = r.ReadUint32()   // 读取循环结束块索引
	tmp, _ := r.ReadUint16()        // 读取 loopR01
	h.loopR01 = uint32(tmp)
	tmp, _ = r.ReadUint16() // 读取 loopR02
	h.loopR02 = uint32(tmp)
	if !(h.loopStart >= 0 && h.loopStart <= h.loopEnd && h.loopEnd < h.blockCount) { // 检查循环范围的有效性
		return false // 无效返回 false
	}
	return true // 读取成功返回 true
}

// ciphHeaderRead 读取 ciph 块的详细信息
func (h *Hca) ciphHeaderRead(r *endibuf.Reader) bool {
	tmp, _ := r.ReadUint16() // 读取 ciphType
	h.ciphType = uint32(tmp)
	if !(h.ciphType == 0 || h.ciphType == 1 || h.ciphType == 0x38) { // 检查 ciphType 的有效值
		return false // 无效返回 false
	}
	return true // 读取成功返回 true
}

// rvaHeaderRead 读取 rva 块的详细信息
func (h *Hca) rvaHeaderRead(r *endibuf.Reader) bool {
	h.rvaVolume, _ = r.ReadFloat32() // 读取 rvaVolume
	return true                      // 读取成功返回 true
}

// commHeaderRead 读取 comm 块的详细信息
func (h *Hca) commHeaderRead(r *endibuf.Reader) bool {
	tmp, _ := r.ReadByte() // 读取注释长度
	h.commLen = uint32(tmp)
	h.commComment, _ = r.ReadCString() // 读取注释字符串（假设 ReadCString 存在并能读取 C 风格字符串）
	return true                        // 读取成功返回 true
}

// ceil2 计算 ceil(a / b)，当 b > 0 时
func ceil2(a, b uint32) uint32 {
	t := uint32(0)
	if b > 0 { // 避免除以零
		t = a / b        // 整数除法
		if (a % b) > 0 { // 如果有余数，结果向上取整
			t++
		}
	}
	return t // 返回计算结果
}
