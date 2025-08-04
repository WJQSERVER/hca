package hca

import "sync"

// Hca 结构体包含了HCA文件的所有解码参数和从文件头解析出的元数据.
type Hca struct {
	// --- 可配置选项 ---
	CiphKey1 uint32  // 解密密钥1
	CiphKey2 uint32  // 解密密钥2
	Mode     int     // 输出WAV格式的位深度 (0=浮点, 8, 16, 24, 32)
	Loop     int     // 强制循环次数, 0表示遵循文件内的循环设置
	Volume   float32 // 音量缩放因子, 1.0为原始音量

	// --- HCA文件元数据 (由loadHeader填充) ---
	version      uint32 // HCA版本
	dataOffset   uint32 // 数据块起始偏移
	channelCount uint32 // 声道数
	samplingRate uint32 // 采样率
	blockCount   uint32 // 总块数
	fmtR01       uint32 // 'fmt' chunk raw data
	fmtR02       uint32 // 'fmt' chunk raw data

	blockSize uint32 // 单个块的大小(字节)
	compR01   uint32 // 'comp'/'dec' chunk raw data
	compR02   uint32 // 'comp'/'dec' chunk raw data
	compR03   uint32 // 'comp'/'dec' chunk raw data
	compR04   uint32 // 'comp'/'dec' chunk raw data
	compR05   uint32 // 'comp'/'dec' chunk raw data
	compR06   uint32 // 'comp'/'dec' chunk raw data
	compR07   uint32 // 'comp'/'dec' chunk raw data
	compR08   uint32 // 'comp'/'dec' chunk raw data
	compR09   uint32 // 'comp'/'dec' chunk raw data

	vbrR01 uint32 // 'vbr' chunk raw data
	vbrR02 uint32 // 'vbr' chunk raw data

	athType uint32 // ATH(音响心理学模型)类型

	loopStart uint32 // 循环起始块
	loopEnd   uint32 // 循环结束块
	loopR01   uint32 // 'loop' chunk raw data
	loopR02   uint32 // 'loop' chunk raw data
	loopFlg   bool   // 是否包含循环信息

	ciphType uint32 // 加密类型

	rvaVolume float32 // RVA(相对音量增益)

	commLen     uint32 // 备注长度
	commComment string // 备注内容

	// --- 内部解码器组件 ---
	ath     stATH           // ATH处理器
	cipher  *Cipher         // 加密处理器
	decoder *channelDecoder // 声道解码器

	// --- 性能优化: 缓冲区池 ---
	// 这些池用于复用在样本格式转换期间的缓冲区, 以减少内存分配.
	int8Pool    sync.Pool
	int16Pool   sync.Pool
	int32Pool   sync.Pool
	float32Pool sync.Pool
	byte24Pool  sync.Pool // 用于24位音频的特殊字节缓冲区
}

// Modes 定义了输出WAV文件的位深度模式.
const (
	ModeFloat = 0  // 32位浮点
	Mode8Bit  = 8  // 8位无符号整数
	Mode16Bit = 16 // 16位有符号整数
	Mode24Bit = 24 // 24位有符号整数 (以3字节存储)
	Mode32Bit = 32 // 32位有符号整数
)

// NewDecoder 创建并返回一个带有默认参数的HCA解码器实例.
func NewDecoder() *Hca {
	return &Hca{
		CiphKey1: 0x30DBE1AB, // 默认解密密钥 1
		CiphKey2: 0xCC554639, // 默认解密密钥 2
		Mode:     Mode16Bit,  // 默认输出 16-bit WAV
		Loop:     0,          // 默认不强制循环
		Volume:   1.0,        // 默认音量
		cipher:   NewCipher(),
	}
}
