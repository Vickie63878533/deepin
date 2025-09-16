package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	defaultOpenAIURL = "https://api.deepinfra.com/v1/openai"
)

var (
	targetURL *url.URL
)

// init 在 main 函数之前执行，用于初始化配置
func init() {
	// 从环境变量获取目标 URL
	openaiURLStr := os.Getenv("OPENAI_URL")
	if openaiURLStr == "" {
		openaiURLStr = defaultOpenAIURL
		log.Printf("OPENAI_URL not set, using default: %s", defaultOpenAIURL)
	}

	var err error
	targetURL, err = url.Parse(openaiURLStr)
	if err != nil {
		log.Fatalf("Error parsing OPENAI_URL '%s': %v", openaiURLStr, err)
	}
	log.Printf("Forwarding requests to: %s", targetURL.String())
}

// userAgents 列表被定义为包级别变量，以提高效率。
// 它包含了各种操作系统和浏览器的 User-Agent 字符串。
var userAgents = []string{
	// --- Windows 10/11 + Chrome ---
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36 Edg/122.0.2365.52",
	"Mozilla/5.0 (Windows NT 11.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",

	// --- Windows 10/11 + Firefox ---
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:125.0) Gecko/20100101 Firefox/125.0",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:124.0) Gecko/20100101 Firefox/124.0",
	"Mozilla/5.0 (Windows NT 11.0; Win64; x64; rv:125.0) Gecko/20100101 Firefox/125.0",

	// --- macOS (OSX) + Safari ---
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4.1 Safari/605.1.15",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.3.1 Safari/605.1.15",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_4_1) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4.1 Safari/605.1.15",

	// --- macOS (OSX) + Chrome ---
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Arm Mac OS X 14_4_1) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",

	// --- Android + Chrome ---
	"Mozilla/5.0 (Linux; Android 14; Pixel 8 Pro) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Mobile Safari/537.36",
	"Mozilla/5.0 (Linux; Android 13; SM-S908B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/112.0.0.0 Mobile Safari/537.36",
	"Mozilla/5.0 (Linux; Android 12; SM-G991U) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Mobile Safari/537.36",
	"Mozilla/5.0 (Linux; Android 11; Pixel 5) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/90.0.4430.91 Mobile Safari/537.36",
	"Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Mobile Safari/537.36",

	// --- iOS + Safari ---
	"Mozilla/5.0 (iPhone; CPU iPhone OS 17_4_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4.1 Mobile/15E148 Safari/604.1",
	"Mozilla/5.0 (iPhone; CPU iPhone OS 17_3_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.3.1 Mobile/15E148 Safari/604.1",
	"Mozilla/5.0 (iPad; CPU OS 17_4_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4.1 Mobile/15E148 Safari/604.1",
	"Mozilla/5.0 (iPod touch; CPU iPhone OS 14_7_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/14.1.2 Mobile/15E148 Safari/604.1",

	// --- iOS + Chrome ---
	"Mozilla/5.0 (iPhone; CPU iPhone OS 17_4 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/124.0.6367.88 Mobile/15E148 Safari/604.1",
	"Mozilla/5.0 (iPhone; CPU iPhone OS 16_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/117.0.5938.108 Mobile/15E148 Safari/604.1",
	"Mozilla/5.0 (iPad; CPU OS 17_4 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/124.0.6367.88 Mobile/15E148 Safari/604.1",
}

// GetRandomUserAgent 从预定义的列表中返回一个随机的 User-Agent。
func GetRandomUserAgent() string {
	// 1. 创建一个新的、使用当前时间作为种子的随机数生成器。
	// 这可以确保每次运行程序时，随机序列都是不同的。
	source := rand.NewSource(time.Now().UnixNano())
	r := rand.New(source)

	// 2. 计算一个随机索引。
	// r.Intn(n) 返回一个 [0, n) 范围内的随机整数。
	// len(userAgents) 是列表的长度。
	randomIndex := r.Intn(len(userAgents))

	// 3. 返回该索引对应的 User-Agent 字符串。
	return userAgents[randomIndex]
}

// GenerateRandomIpFromCidr 从一个给定的 CIDR 块中生成一个随机的IPv4地址。
// 它返回生成的IP地址字符串和一个错误（如果发生错误）。
func GenerateRandomIpFromCidr(cidr string) (string, error) {
	// --- 1. 解析和验证 CIDR ---
	// net.ParseCIDR 会同时返回 IP 地址和子网信息 (ipnet)。
	// 如果 CIDR 格式无效，它会返回一个错误。
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", fmt.Errorf("无效的 CIDR 格式: %w", err)
	}

	// 确保是 IPv4 地址
	ipv4 := ip.To4()
	if ipv4 == nil {
		return "", errors.New("仅支持 IPv4 CIDR")
	}

	// --- 2. 计算范围 ---
	// ipnet.Mask.Size() 返回前缀长度 (例如，/24 返回 24, 32)
	prefixLen, _ := ipnet.Mask.Size()
	if prefixLen == 32 {
		return ip.String(), nil // 如果是 /32，只有一个地址，直接返回
	}

	// 主机部分的位数
	hostBits := 32 - prefixLen
	// 主机部分可以生成的地址数量 (2 的 hostBits 次方)
	// 使用 1 << hostBits (位移操作) 比 math.Pow 更高效
	numHosts := uint32(1 << hostBits)

	// --- 3. 将网络地址转换为整数 ---
	// binary.BigEndian.Uint32 将 4 字节的 IP 地址转换为 uint32 整数。
	// 我们操作的是网络地址，即 CIDR 块的起始地址。
	startIpInt := binary.BigEndian.Uint32(ipnet.IP.To4())

	// --- 4. 生成随机偏移量 ---
	// 创建一个独立的随机数源，以确保并发安全和良好的随机性。
	source := rand.NewSource(time.Now().UnixNano())
	r := rand.New(source)
	randomOffset := r.Uint32() % numHosts // 确保偏移量在有效范围内

	// --- 5. 计算新 IP ---
	randomIpInt := startIpInt + randomOffset

	// --- 6. 转换回 IP 字符串并返回 ---
	randomIpBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(randomIpBytes, randomIpInt)
	randomIP := net.IP(randomIpBytes)

	return randomIP.String(), nil
}

// handleRequest 是主要的 HTTP 请求处理器
func handleRequest(w http.ResponseWriter, r *http.Request) {
	// 获取请求路径
	path := r.URL.Path

	// 检查是否为根路径或空路径的直接访问
	if !strings.HasPrefix(path, "/v1") {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("OK"))
		if err != nil {
			return 
		}
		return
	}

	path = strings.Replace(path, "/v1", "", 1)

	// 选择 API 密钥
	apiKey := ""

	// 创建反向代理
	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	// 自定义 Director 函数来修改请求
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req) // 执行默认的 Director 逻辑 (如设置 X-Forwarded-For 等)

		// 设置目标请求的 URL scheme, host 和 path
		req.URL.Scheme = targetURL.Scheme
		req.URL.Host = targetURL.Host
		req.URL.Path = targetURL.Path + path // 使用原始请求的路径拼接到目标域名后

		// 修改 Host 头部
		req.Host = targetURL.Host

		// 设置 Authorization 头部
		req.Header.Set("Authorization", "Bearer "+apiKey)

		req.Header.Del("Cf-Connecting-Ip")
		req.Header.Del("Cf-Ipcountry")
		req.Header.Del("Cf-Visitor")
		req.Header.Del("X-Forwarded-Proto")
		req.Header.Del("X-Real-Ip")
		req.Header.Del("X-Forwarded-For")
		req.Header.Del("X-Forwarded-Port")
		req.Header.Del("X-Stainless-Arch")
		req.Header.Del("X-Stainless-Package-Version")
		req.Header.Del("X-Direct-Url")
		req.Header.Del("X-Middleware-Subrequest")
		req.Header.Del("X-Stainless-Runtime")
		req.Header.Del("X-Stainless-Lang")
		randomIP, err := GenerateRandomIpFromCidr("32.250.0.0/14")
		if err != nil {
			fmt.Printf("生成失败: %v\n", err)
			req.Header.Set("X-Real-IP", randomIP)
		} else {

		}
		req.Header.Set("User-Agent", GetRandomUserAgent())

		//log.Printf("Forwarding request: %s %s%s to %s%s", req.Method, req.Host, req.URL.Path, targetURL.Scheme, targetURL.Host+path)
	}

	proxy.ModifyResponse = func(resp *http.Response) error {
		// resp.Request is the *outgoing* request that was sent to the target.
		// This request object should contain the context we set in the Director.
		if resp.Request == nil {
			log.Println("WARN: ModifyResponse: resp.Request is nil. Cannot check for API key context.")
			return nil // Nothing to do if we don't have the original request.
		}

		if resp.StatusCode == http.StatusForbidden { // 403
			log.Printf("INFO: ModifyResponse: Upstream returned 403 for key: '%s'. Attempting to remove it.", apiKey)
			return nil
		}

		if resp.StatusCode == http.StatusUnprocessableEntity { // 422
			log.Printf("INFO: ModifyResponse: Upstream returned 422 for key: '%s'. Attempting to remove it.", apiKey)
			return nil
		}
		return nil // Return nil to indicate no error during response modification
	}

	// 可选：自定义 ErrorHandler
	proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
		log.Printf("Proxy error: %v", err)
		http.Error(rw, "Error forwarding request.", http.StatusBadGateway)
	}

	// 执行转发
	proxy.ServeHTTP(w, r)
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // 默认端口
		log.Printf("PORT environment variable not set, using default %s", port)
	}

	http.HandleFunc("/", handleRequest)

	server := &http.Server{
		Addr:         ":" + port,
		ReadTimeout:  300 * time.Second,
		WriteTimeout: 300 * time.Second, // 对于流式响应，可能需要更长或无超时
		IdleTimeout:  600 * time.Second,
	}

	log.Printf("Starting server on port %s...", port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Could not listen on %s: %v\n", port, err)
	}
}
