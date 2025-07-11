package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings" // 用于ToLower
	"sync"

	"github.com/WJQSERVER/hca" // 保持原始库的导入
)

// global flags
var (
	saveDirFlag  *string
	ciphKey1Flag *uint // 使用 uint 因为 flag 包没有 uint32, 但解析十六进制时会处理
	ciphKey2Flag *uint
	modeFlag     *int
	loopFlag     *int
	volumeFlag   *float64
	parallelFlag *int
)

func init() {
	// 使用空字符串作为默认值，表示与源文件同目录
	saveDirFlag = flag.String("save", "", "保存WAV文件的目录 (默认为源文件所在目录)")
	ciphKey1Flag = flag.Uint("c1", 0x01395C51, "解密密钥1 (十六进制, 例如 0x01395C51)")
	ciphKey2Flag = flag.Uint("c2", 0x00000000, "解密密钥2 (十六进制, 例如 0x00000000)")
	modeFlag = flag.Int("m", 16, "解码输出位数 (0=浮点, 8, 16, 24, 32)")
	loopFlag = flag.Int("l", 0, "循环次数 (0=使用文件内设置, >0=强制循环N次)")
	volumeFlag = flag.Float64("v", 1.0, "音量缩放 (例如 0.5, 1.0, 1.5)")
	parallelFlag = flag.Int("p", runtime.NumCPU(), "并行解码的文件数量 (默认为CPU核心数)")

	// 自定义 Usage 函数
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "HCA 文件解码器 (基于 go-hca 库)\n\n")
		fmt.Fprintf(os.Stderr, "用法: %s [选项] <hca文件1> [hca文件2] ...\n\n", filepath.Base(os.Args[0]))
		fmt.Fprintf(os.Stderr, "选项:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\n示例:\n")
		fmt.Fprintf(os.Stderr, "  %s song.hca\n", filepath.Base(os.Args[0]))
		fmt.Fprintf(os.Stderr, "  %s -save ./decoded_audio -m 0 -v 1.2 music1.hca sound_effect.hca\n", filepath.Base(os.Args[0]))
	}
}

func main() {
	log.SetFlags(0) // 不显示日期时间前缀
	flag.Parse()

	filesToProcess := flag.Args()
	if len(filesToProcess) == 0 {
		log.Println("错误: 请提供至少一个HCA文件进行解码。")
		flag.Usage()
		os.Exit(1)
	}

	numParallel := *parallelFlag
	if numParallel <= 0 {
		numParallel = 1 // 至少一个任务
	}
	if numParallel > len(filesToProcess) { // 并行数不需要超过文件数
		numParallel = len(filesToProcess)
	}

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, numParallel) // 控制并发数量的信号量

	log.Printf("开始解码 %d 个文件，并行数: %d\n", len(filesToProcess), numParallel)

	for _, hcaFilePath := range filesToProcess {
		wg.Add(1)
		semaphore <- struct{}{} // 获取一个处理许可

		go func(inputFile string) {
			defer wg.Done()
			defer func() { <-semaphore }() // 释放许可

			processFile(inputFile)
		}(hcaFilePath)
	}

	wg.Wait() // 等待所有文件处理完毕
	log.Println("所有解码任务完成。")
}

func processFile(hcaFilePath string) {
	// 基本的文件有效性检查
	if _, err := os.Stat(hcaFilePath); os.IsNotExist(err) {
		log.Printf("错误: 文件不存在 %s", hcaFilePath)
		return
	}
	if strings.ToLower(filepath.Ext(hcaFilePath)) != ".hca" {
		log.Printf("跳过: %s (非 .hca 文件)", hcaFilePath)
		return
	}

	// 创建和配置解码器实例
	// 由于库的 Decoder 状态不是线程安全的（如果它内部有可变状态用于解码单个文件），
	// 并且我们的并发模型是每个文件一个goroutine，所以每个goroutine都应有自己的Decoder实例。
	decoder := hca.NewDecoder() // 使用库提供的构造函数
	decoder.CiphKey1 = uint32(*ciphKey1Flag)
	decoder.CiphKey2 = uint32(*ciphKey2Flag)
	decoder.Mode = *modeFlag
	decoder.Loop = *loopFlag
	decoder.Volume = float32(*volumeFlag)

	// 准备输出文件名和路径
	outputBaseName := hcaFilePath[:len(hcaFilePath)-len(filepath.Ext(hcaFilePath))] + ".wav"
	var outputFilePath string

	if *saveDirFlag != "" { // 如果指定了输出目录
		// 确保输出目录存在
		if err := os.MkdirAll(*saveDirFlag, 0755); err != nil {
			log.Printf("错误: 无法创建目录 '%s': %v (文件: %s)", *saveDirFlag, err, hcaFilePath)
			return
		}
		outputFilePath = filepath.Join(*saveDirFlag, filepath.Base(outputBaseName))
	} else { // 否则，输出到源文件相同目录
		outputFilePath = outputBaseName
	}

	// 执行解码
	log.Printf("正在处理: %s -> %s", hcaFilePath, outputFilePath)
	success := decoder.DecodeFromFile(hcaFilePath, outputFilePath) // 库函数返回 bool

	if success {
		log.Printf("成功解码: %s", outputFilePath)
	} else {
		// 库本身在 DecodeFromFile 失败时会删除目标文件，所以这里不需要额外删除
		log.Printf("解码失败: %s (检查库的内部错误或文件是否损坏)", hcaFilePath)
		// 由于库不返回具体错误，我们只能给出通用提示
	}
}
