package hca

import (
	"github.com/vazrupe/endibuf"
)

// Hca is Hca File Structor
// Hca 是 HCA 文件结构体
type Hca struct {
	CiphKey1 uint32 // 密码密钥 1
	CiphKey2 uint32 // 密码密钥 2

	Mode int // 写入模式（例如 16 位）
	Loop int // 循环次数

	Volume float32 // 音量

	version    uint32 // 版本
	dataOffset uint32 // 数据偏移量

	channelCount uint32 // 通道数量
	samplingRate uint32 // 采样率
	blockCount   uint32 // 块总数
	fmtR01       uint32 // fmt chunk 中的 R01 字段
	fmtR02       uint32 // fmt chunk 中的 R02 字段

	blockSize uint32 // 块大小
	compR01   uint32 // comp chunk 中的 R01 字段
	compR02   uint32 // comp chunk 中的 R02 字段
	compR03   uint32 // comp chunk 中的 R03 字段
	compR04   uint32 // comp chunk 中的 R04 字段
	compR05   uint32 // comp chunk 中的 R05 字段
	compR06   uint32 // comp chunk 中的 R06 字段
	compR07   uint32 // comp chunk 中的 R07 字段
	compR08   uint32 // comp chunk 中的 R08 字段
	compR09   uint32 // comp chunk 中的 R09 字段

	vbrR01 uint32 // vbr chunk 中的 R01 字段
	vbrR02 uint32 // vbr chunk 中的 R02 字段

	athType uint32 // ATH 类型

	loopStart uint32 // 循环开始块索引
	loopEnd   uint32 // 循环结束块索引
	loopR01   uint32 // loop chunk 中的 R01 字段
	loopR02   uint32 // loop chunk 中的 R02 字段
	loopFlg   bool   // 循环标志

	ciphType uint32 // 密码类型

	rvaVolume float32 // 相对音量调整

	commLen     uint32 // 注释长度
	commComment string // 注释内容

	ath    stATH   // ATH 数据结构（假设 stATH 已定义）
	cipher *Cipher // 密码对象（假设 Cipher 已定义）

	decoder *channelDecoder // 通道解码器（假设 channelDecoder 已定义）

	saver func(f float32, w *endibuf.Writer) // 保存函数，用于将浮点样本写入 endibuf.Writer
}

// Modes is writting mode num
// Modes 是写入模式编号
const (
	ModeFloat = 0  // 浮点模式
	Mode8Bit  = 8  // 8 位模式
	Mode16Bit = 16 // 16 位模式
	Mode24Bit = 24 // 24 位模式
	Mode32Bit = 32 // 32 位模式
)

// NewDecoder is create hca with default option
// NewDecoder 使用默认选项创建 HCA 解码器
func NewDecoder() *Hca {
	return &Hca{CiphKey1: 0x30DBE1AB, // 默认密码密钥 1
		CiphKey2: 0xCC554639,  // 默认密码密钥 2
		Mode:     16,          // 默认模式为 16 位
		Loop:     0,           // 默认循环次数为 0
		Volume:   1.0,         // 默认音量为 1.0
		cipher:   NewCipher()} // 创建新的密码对象
}
