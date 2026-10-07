package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// MediaInfo 描述一个视频文件的容器、视频流与音频流信息，用于在界面上展示原始编码信息。
type MediaInfo struct {
	Path           string     `json:"path"`
	FileName       string     `json:"fileName"`
	SizeBytes      int64      `json:"sizeBytes"`
	SizeText       string     `json:"sizeText"`
	DurationSec    float64    `json:"durationSec"`
	DurationText   string     `json:"durationText"`
	Container      string     `json:"container"`
	ContainerLong  string     `json:"containerLong"`
	BitRateText    string     `json:"bitRateText"`
	Video          *VideoInfo `json:"video"`
	Audio          *AudioInfo `json:"audio"`
	AudioTracks    int        `json:"audioTracks"`
	SubtitleTracks int        `json:"subtitleTracks"`
	Warnings       []string   `json:"warnings"`
	// AudioCodecs 列出全部音频流的编码名（小写）。
	AudioCodecs []string `json:"audioCodecs"`
	// AudioCompatible 表示所有音频流都能直接 copy 进 MP4 容器。
	AudioCompatible bool `json:"audioCompatible"`
	// BitmapSubtitles 列出 MP4 无法承载的图形字幕编码。
	BitmapSubtitles []string `json:"bitmapSubtitles"`
}

// VideoInfo 描述视频流的关键编码参数。
type VideoInfo struct {
	Index        int    `json:"index"`
	Codec        string `json:"codec"`
	CodecLong    string `json:"codecLong"`
	Profile      string `json:"profile"`
	Level        string `json:"level"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Resolution   string `json:"resolution"`
	AspectRatio  string `json:"aspectRatio"`
	PixelFormat  string `json:"pixelFormat"`
	BitDepth     string `json:"bitDepth"`
	FrameRate    string `json:"frameRate"`
	BitRateText  string `json:"bitRateText"`
	ColorInfo    string `json:"colorInfo"`
	ScanType     string `json:"scanType"`
	DurationText string `json:"durationText"`
	// RecommendedLevel 是编码该分辨率/帧率所需的最低 H.264 等级。
	RecommendedLevel     string `json:"recommendedLevel"`
	RecommendedLevelHint string `json:"recommendedLevelHint"`
}

// AudioInfo 描述音频流的关键编码参数。
type AudioInfo struct {
	Index         int    `json:"index"`
	Codec         string `json:"codec"`
	CodecLong     string `json:"codecLong"`
	Profile       string `json:"profile"`
	SampleRate    string `json:"sampleRate"`
	Channels      int    `json:"channels"`
	ChannelLayout string `json:"channelLayout"`
	BitRateText   string `json:"bitRateText"`
	SampleFormat  string `json:"sampleFormat"`
}

// ---- ffprobe JSON 结构 ----

type ffprobeResult struct {
	Streams []ffprobeStream `json:"streams"`
	Format  ffprobeFormat   `json:"format"`
}

type ffprobeFormat struct {
	FileName       string `json:"filename"`
	FormatName     string `json:"format_name"`
	FormatLongName string `json:"format_long_name"`
	Duration       string `json:"duration"`
	Size           string `json:"size"`
	BitRate        string `json:"bit_rate"`
}

type ffprobeStream struct {
	Index            int    `json:"index"`
	CodecName        string `json:"codec_name"`
	CodecLongName    string `json:"codec_long_name"`
	CodecType        string `json:"codec_type"`
	Profile          string `json:"profile"`
	Level            int    `json:"level"`
	Width            int    `json:"width"`
	Height           int    `json:"height"`
	DisplayAspectRat string `json:"display_aspect_ratio"`
	PixFmt           string `json:"pix_fmt"`
	BitsPerRawSample string `json:"bits_per_raw_sample"`
	RFrameRate       string `json:"r_frame_rate"`
	AvgFrameRate     string `json:"avg_frame_rate"`
	BitRate          string `json:"bit_rate"`
	SampleRate       string `json:"sample_rate"`
	Channels         int    `json:"channels"`
	ChannelLayout    string `json:"channel_layout"`
	SampleFmt        string `json:"sample_fmt"`
	FieldOrder       string `json:"field_order"`
	ColorSpace       string `json:"color_space"`
	ColorTransfer    string `json:"color_transfer"`
	ColorPrimaries   string `json:"color_primaries"`
	Duration         string `json:"duration"`
}

// probeMedia 调用 ffprobe 读取文件的媒体信息。
func probeMedia(ctx context.Context, path string) (*MediaInfo, error) {
	ffprobe := locateTool("ffprobe")
	if ffprobe == "" {
		return nil, errors.New("未找到 ffprobe，请安装 FFmpeg 并将 bin 目录加入 PATH，或设置 FFPROBE_PATH 环境变量")
	}

	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cctx, ffprobe,
		"-hide_banner", "-v", "error",
		"-print_format", "json",
		"-show_format", "-show_streams",
		path,
	)
	cmd.SysProcAttr = hideWindow()

	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			detail := strings.TrimSpace(string(exitErr.Stderr))
			if detail != "" {
				return nil, fmt.Errorf("ffprobe 读取失败：%s", firstLines(detail, 3))
			}
		}
		return nil, fmt.Errorf("ffprobe 读取失败：%w", err)
	}

	var raw ffprobeResult
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("解析 ffprobe 输出失败：%w", err)
	}

	return buildMediaInfo(path, &raw), nil
}

func buildMediaInfo(path string, raw *ffprobeResult) *MediaInfo {
	info := &MediaInfo{
		Path:            path,
		FileName:        filepath.Base(path),
		Container:       raw.Format.FormatName,
		ContainerLong:   raw.Format.FormatLongName,
		DurationSec:     parseFloat(raw.Format.Duration),
		SizeBytes:       int64(parseFloat(raw.Format.Size)),
		Warnings:        []string{},
		AudioCodecs:     []string{},
		AudioCompatible: true,
	}

	if st, err := os.Stat(path); err == nil && info.SizeBytes == 0 {
		info.SizeBytes = st.Size()
	}

	info.DurationText = formatDuration(info.DurationSec)
	info.SizeText = formatBytes(info.SizeBytes)

	// 总体码率：容器未提供时按 文件大小 / 时长 估算。
	bitRate := parseFloat(raw.Format.BitRate)
	if bitRate <= 0 && info.DurationSec > 0 && info.SizeBytes > 0 {
		bitRate = float64(info.SizeBytes) * 8 / info.DurationSec
	}
	info.BitRateText = formatBitRate(bitRate)

	for i := range raw.Streams {
		s := &raw.Streams[i]
		switch s.CodecType {
		case "video":
			// 跳过封面图等附属图片流。
			if info.Video == nil && s.CodecName != "mjpeg" && s.CodecName != "png" && s.CodecName != "bmp" {
				info.Video = buildVideoInfo(s)
			}
		case "audio":
			info.AudioTracks++
			if s.CodecName != "" {
				info.AudioCodecs = append(info.AudioCodecs, strings.ToLower(s.CodecName))
				if !mp4AudioOK[strings.ToLower(s.CodecName)] {
					info.AudioCompatible = false
				}
			}
			if info.Audio == nil {
				info.Audio = buildAudioInfo(s)
			}
		case "subtitle":
			info.SubtitleTracks++
			if isBitmapSubtitle(s.CodecName) {
				info.BitmapSubtitles = append(info.BitmapSubtitles, strings.ToLower(s.CodecName))
			}
		}
	}

	// 视频流未给出码率时，用总码率减去音频码率估算。
	if info.Video != nil && info.Video.BitRateText == "" && bitRate > 0 {
		audioBits := float64(0)
		for i := range raw.Streams {
			if raw.Streams[i].CodecType == "audio" {
				audioBits += parseFloat(raw.Streams[i].BitRate)
			}
		}
		if audioBits > 0 && bitRate > audioBits {
			info.Video.BitRateText = formatBitRate(bitRate - audioBits)
		} else {
			info.Video.BitRateText = formatBitRate(bitRate)
		}
	}

	info.Warnings = buildWarnings(info)
	return info
}

func buildVideoInfo(s *ffprobeStream) *VideoInfo {
	v := &VideoInfo{
		Index:       s.Index,
		Codec:       strings.ToUpper(s.CodecName),
		CodecLong:   s.CodecLongName,
		Profile:     s.Profile,
		Level:       codecLevelText(s.CodecName, s.Level),
		Width:       s.Width,
		Height:      s.Height,
		PixelFormat: s.PixFmt,
		FrameRate:   formatFrameRate(s.AvgFrameRate, s.RFrameRate),
		BitRateText: formatBitRate(parseFloat(s.BitRate)),
		ScanType:    scanTypeText(s.FieldOrder),
		BitDepth:    bitDepthText(s.PixFmt, s.BitsPerRawSample),
	}
	if s.Width > 0 && s.Height > 0 {
		v.Resolution = fmt.Sprintf("%d × %d", s.Width, s.Height)
	}
	v.AspectRatio = s.DisplayAspectRat
	if v.AspectRatio == "" && s.Width > 0 && s.Height > 0 {
		v.AspectRatio = reduceRatio(s.Width, s.Height)
	}
	v.ColorInfo = joinNonEmpty(" / ", s.ColorSpace, s.ColorTransfer, s.ColorPrimaries)
	v.DurationText = formatDuration(parseFloat(s.Duration))
	v.RecommendedLevel, v.RecommendedLevelHint = recommendLevel(s.Width, s.Height, frameRateValue(s.AvgFrameRate, s.RFrameRate))
	return v
}

func buildAudioInfo(s *ffprobeStream) *AudioInfo {
	layout := s.ChannelLayout
	if layout == "" && s.Channels > 0 {
		layout = defaultChannelLayout(s.Channels)
	}
	return &AudioInfo{
		Index:         s.Index,
		Codec:         strings.ToUpper(s.CodecName),
		CodecLong:     s.CodecLongName,
		Profile:       s.Profile,
		SampleRate:    formatSampleRate(s.SampleRate),
		Channels:      s.Channels,
		ChannelLayout: layout,
		BitRateText:   formatBitRate(parseFloat(s.BitRate)),
		SampleFormat:  s.SampleFmt,
	}
}

// buildWarnings 依据探测结果提示本次转码可能遇到的问题。
func buildWarnings(info *MediaInfo) []string {
	warnings := make([]string, 0, 4)

	if info.Video == nil {
		return append(warnings, "该文件没有可识别的视频流，无法转码。")
	}

	if !mp4Containers[strings.ToLower(filepath.Ext(info.Path))] {
		warnings = append(warnings, fmt.Sprintf(
			"源文件是 %s 容器，输出将改为 MP4（H.264 + faststart，便于网页播放）。",
			strings.TrimPrefix(strings.ToLower(filepath.Ext(info.Path)), ".")))
	}

	if info.AudioTracks > 0 && !info.AudioCompatible {
		warnings = append(warnings, fmt.Sprintf(
			"音频编码 %s 与 MP4 容器不兼容，-c:a copy 会失败，将自动改为 AAC 128k。",
			strings.ToUpper(strings.Join(info.AudioCodecs, ", "))))
	}

	// level 是硬性上限：按宏块数与宏块率算出的最低等级如果高于默认档，就必须调整。
	if rec := info.Video.RecommendedLevel; rec != "" && levelValue(rec) > levelValue(defaultLevel) {
		warnings = append(warnings, fmt.Sprintf(
			"按 %s / %s 计算，H.264 至少需要 level %s（%s）；默认 level %s 会超出上限导致 ffmpeg 报错，请在「高级参数」中改选 level %s 或「自动」。",
			info.Video.Resolution, info.Video.FrameRate, rec, info.Video.RecommendedLevelHint,
			defaultLevel, rec))
	}

	if info.Video.Width%2 != 0 || info.Video.Height%2 != 0 {
		warnings = append(warnings, fmt.Sprintf(
			"分辨率 %s 含奇数边长，yuv420p 要求宽高均为偶数，转码可能失败。", info.Video.Resolution))
	}

	if len(info.BitmapSubtitles) > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"源文件含图形字幕（%s），MP4 无法承载，将自动加 -sn 丢弃字幕。",
			strings.ToUpper(strings.Join(info.BitmapSubtitles, ", "))))
	}

	if strings.EqualFold(info.Video.Codec, "h264") &&
		strings.EqualFold(info.Video.Profile, "main") &&
		strings.EqualFold(info.Video.PixelFormat, "yuv420p") {
		warnings = append(warnings, "源视频已经是 H.264 / Main / yuv420p，重编码的意义主要在于按 CRF 精确控制体积与画质。")
	}

	return warnings
}

var bitmapSubtitles = map[string]bool{
	"hdmv_pgs_subtitle": true,
	"dvd_subtitle":      true,
	"dvb_subtitle":      true,
	"xsub":              true,
}

func isBitmapSubtitle(codec string) bool {
	return bitmapSubtitles[strings.ToLower(codec)]
}

// ---- 格式化辅助 ----

func formatBytes(n int64) string {
	if n <= 0 {
		return ""
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"KB", "MB", "GB", "TB"}
	value := float64(n)
	for _, u := range units {
		value /= unit
		if value < unit {
			if value < 10 {
				return fmt.Sprintf("%.2f %s", value, u)
			}
			return fmt.Sprintf("%.1f %s", value, u)
		}
	}
	return fmt.Sprintf("%.1f PB", value/unit)
}

func formatDuration(sec float64) string {
	if sec <= 0 {
		return ""
	}
	total := int(sec)
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

func formatBitRate(bits float64) string {
	if bits <= 0 {
		return ""
	}
	if bits >= 1_000_000 {
		return fmt.Sprintf("%.2f Mb/s", bits/1_000_000)
	}
	if bits >= 1_000 {
		return fmt.Sprintf("%.0f kb/s", bits/1_000)
	}
	return fmt.Sprintf("%.0f b/s", bits)
}

func formatSampleRate(s string) string {
	v := parseFloat(s)
	if v <= 0 {
		return ""
	}
	if v >= 1000 {
		return fmt.Sprintf("%.1f kHz", v/1000)
	}
	return fmt.Sprintf("%.0f Hz", v)
}

func formatFrameRate(rates ...string) string {
	fps := frameRateValue(rates...)
	if fps <= 0 {
		return ""
	}
	if fps == float64(int(fps)) {
		return fmt.Sprintf("%d fps", int(fps))
	}
	return fmt.Sprintf("%.3f fps", fps)
}

// frameRateValue 把 "30000/1001" 之类的帧率表达式转成数值。
func frameRateValue(rates ...string) float64 {
	for _, r := range rates {
		if r == "" || r == "0/0" {
			continue
		}
		parts := strings.SplitN(r, "/", 2)
		if len(parts) == 2 {
			num := parseFloat(parts[0])
			den := parseFloat(parts[1])
			if den <= 0 || num <= 0 {
				continue
			}
			return num / den
		}
		if v := parseFloat(r); v > 0 {
			return v
		}
	}
	return 0
}

func codecLevelText(codec string, level int) string {
	if level <= 0 {
		return ""
	}
	switch strings.ToLower(codec) {
	case "hevc", "h265":
		// HEVC 的 level 是 general_level_idc，为实际等级的 30 倍。
		return fmt.Sprintf("%d.%d", level/30, (level%30)/3)
	case "h264", "avc1":
		return fmt.Sprintf("%d.%d", level/10, level%10)
	default:
		return strconv.Itoa(level)
	}
}

func bitDepthText(pixFmt, raw string) string {
	if raw != "" && raw != "0" {
		return raw + " bit"
	}
	switch {
	case pixFmt == "":
		return ""
	case strings.HasPrefix(pixFmt, "nv12"), strings.HasPrefix(pixFmt, "nv21"):
		return "8 bit"
	case strings.Contains(pixFmt, "16"):
		return "16 bit"
	case strings.Contains(pixFmt, "12"):
		return "12 bit"
	case strings.Contains(pixFmt, "10"):
		return "10 bit"
	case strings.Contains(pixFmt, "9"):
		return "9 bit"
	}
	return "8 bit"
}

func scanTypeText(fieldOrder string) string {
	switch strings.ToLower(fieldOrder) {
	case "progressive":
		return "逐行扫描"
	case "tt", "bb", "tb", "bt":
		return "隔行扫描（" + strings.ToUpper(fieldOrder) + "）"
	}
	return ""
}

func defaultChannelLayout(channels int) string {
	switch channels {
	case 1:
		return "mono"
	case 2:
		return "stereo"
	case 6:
		return "5.1"
	case 8:
		return "7.1"
	}
	return fmt.Sprintf("%d 声道", channels)
}

func reduceRatio(w, h int) string {
	g := gcd(w, h)
	if g <= 0 {
		return ""
	}
	return fmt.Sprintf("%d:%d", w/g, h/g)
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

func joinNonEmpty(sep string, parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, sep)
}

func parseFloat(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return v
}

func firstLines(s string, n int) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, " ")
}
