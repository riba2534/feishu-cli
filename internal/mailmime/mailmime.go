// Package mailmime 提供"最小改动"的 EML 解析与序列化，用于邮件草稿编辑（mail draft-edit）。
//
// 设计目标是读-改-写时尽量保持原 EML 不变：
//   - 头字段按原始顺序、原始折行保存，未修改的头原样写回（In-Reply-To、References、
//     X-LMS-Reply-To-Message-Id、Message-Id 等都不会丢）；
//   - multipart 结构（boundary、preamble、epilogue、子 part）原样保留，未修改的 part
//     （附件、内联图片）连同其传输编码一起原样写回；
//   - 只有被显式修改的头或正文 part 才会重新生成。
//
// 不追求完整的 RFC 5322/2045 实现：解析失败时调用方应报错而不是退化为全量重建。
package mailmime

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/quotedprintable"
	"strings"
)

// Field 一个头字段：Name 为字段名，Raw 为完整原始文本（含折行，不含结尾换行）。
type Field struct {
	Name string
	Raw  string
}

// Value 返回去折行后的字段值（不做 RFC 2047 解码）。
func (f Field) Value() string {
	idx := strings.IndexByte(f.Raw, ':')
	if idx < 0 {
		return ""
	}
	v := f.Raw[idx+1:]
	v = strings.ReplaceAll(v, "\r\n", "")
	v = strings.ReplaceAll(v, "\n", "")
	return strings.TrimSpace(v)
}

// Part 一个 MIME 实体（顶层邮件本身也是一个 Part）。
type Part struct {
	Fields []Field

	// 叶子 part：Body 为原始（仍是传输编码后的）内容。
	Body []byte

	// multipart：Boundary 非空，Children 为子 part；Preamble/Epilogue 原样保留。
	Boundary    string
	Children    []*Part
	Preamble    []byte
	Epilogue    []byte
	hasPreamble bool // 第一个分隔行前有内容（哪怕是空行）
	hasEpilogue bool // 结束分隔行后有内容（哪怕只是一个换行）

	eol string // 换行符（"\r\n" 或 "\n"），沿用原文
}

// Parse 解析原始 EML。
func Parse(raw []byte) (*Part, error) {
	eol := "\n"
	if bytes.Contains(raw, []byte("\r\n")) {
		eol = "\r\n"
	}
	return parsePart(raw, eol, 0)
}

const maxDepth = 32

func parsePart(raw []byte, eol string, depth int) (*Part, error) {
	if depth > maxDepth {
		return nil, fmt.Errorf("MIME 嵌套层级超过 %d", maxDepth)
	}
	p := &Part{eol: eol}
	sep := []byte(eol + eol)
	var headerBlock, body []byte
	switch {
	case bytes.HasPrefix(raw, []byte(eol)):
		// 没有头字段，直接是正文
		body = raw[len(eol):]
	default:
		if idx := bytes.Index(raw, sep); idx >= 0 {
			headerBlock = raw[:idx]
			body = raw[idx+len(sep):]
		} else {
			headerBlock = bytes.TrimSuffix(raw, []byte(eol))
		}
	}
	fields, err := parseFields(string(headerBlock), eol)
	if err != nil {
		return nil, err
	}
	p.Fields = fields

	mediaType, params := p.MediaType()
	if strings.HasPrefix(mediaType, "multipart/") && params["boundary"] != "" {
		p.Boundary = params["boundary"]
		if err := p.splitMultipart(body, depth); err != nil {
			return nil, err
		}
		return p, nil
	}
	p.Body = body
	return p, nil
}

func parseFields(block, eol string) ([]Field, error) {
	if strings.TrimSpace(block) == "" {
		return nil, nil
	}
	lines := strings.Split(block, eol)
	var fields []Field
	for _, line := range lines {
		if line == "" {
			continue
		}
		if line[0] == ' ' || line[0] == '\t' {
			if len(fields) == 0 {
				return nil, fmt.Errorf("头字段折行出现在第一个字段之前")
			}
			fields[len(fields)-1].Raw += eol + line
			continue
		}
		idx := strings.IndexByte(line, ':')
		if idx <= 0 {
			return nil, fmt.Errorf("无法解析的头字段行: %q", truncate(line, 80))
		}
		fields = append(fields, Field{Name: strings.TrimSpace(line[:idx]), Raw: line})
	}
	return fields, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// splitMultipart 按 boundary 切分子 part。按 RFC 2046，分隔行之前的换行属于分隔符本身。
func (p *Part) splitMultipart(body []byte, depth int) error {
	eol := p.eol
	delim := "--" + p.Boundary
	lines := strings.Split(string(body), eol)
	var (
		current   []string
		inPart    bool
		closed    bool
		preamble  []string
		epilogue  []string
		partsText [][]string
	)
	for _, line := range lines {
		if closed {
			epilogue = append(epilogue, line)
			continue
		}
		trimmed := strings.TrimRight(line, " \t")
		switch trimmed {
		case delim + "--":
			if inPart {
				partsText = append(partsText, current)
			}
			current = nil
			inPart = false
			closed = true
			continue
		case delim:
			if inPart {
				partsText = append(partsText, current)
			}
			current = nil
			inPart = true
			continue
		}
		if inPart {
			current = append(current, line)
		} else {
			preamble = append(preamble, line)
		}
	}
	if !closed {
		// 缺少结束分隔符：容忍，把最后一个 part 收进来
		if inPart {
			partsText = append(partsText, current)
		}
	}
	if len(partsText) == 0 {
		return fmt.Errorf("multipart 实体中没有找到 boundary %q 分隔的子 part", p.Boundary)
	}
	if len(preamble) > 0 {
		p.hasPreamble = true
		p.Preamble = []byte(strings.Join(preamble, eol))
	}
	if len(epilogue) > 0 {
		p.hasEpilogue = true
		p.Epilogue = []byte(strings.Join(epilogue, eol))
	}
	for _, pt := range partsText {
		child, err := parsePart([]byte(strings.Join(pt, eol)), eol, depth+1)
		if err != nil {
			return err
		}
		p.Children = append(p.Children, child)
	}
	return nil
}

// Bytes 序列化为 EML。
func (p *Part) Bytes() []byte {
	var b bytes.Buffer
	p.write(&b)
	return b.Bytes()
}

func (p *Part) write(b *bytes.Buffer) {
	eol := p.eol
	if eol == "" {
		eol = "\r\n"
	}
	for _, f := range p.Fields {
		b.WriteString(f.Raw)
		b.WriteString(eol)
	}
	b.WriteString(eol)
	if p.Boundary == "" {
		b.Write(p.Body)
		return
	}
	if p.hasPreamble || len(p.Preamble) > 0 {
		b.Write(p.Preamble)
		b.WriteString(eol)
	}
	for i, c := range p.Children {
		if i > 0 {
			b.WriteString(eol)
		}
		b.WriteString("--" + p.Boundary + eol)
		if c.eol == "" {
			c.eol = eol
		}
		c.write(b)
	}
	b.WriteString(eol + "--" + p.Boundary + "--")
	if p.hasEpilogue || len(p.Epilogue) > 0 {
		b.WriteString(eol)
		b.Write(p.Epilogue)
	}
}

// Get 返回第一个同名头字段的去折行值（大小写不敏感）；不存在返回空串。
func (p *Part) Get(name string) string {
	for _, f := range p.Fields {
		if strings.EqualFold(f.Name, name) {
			return f.Value()
		}
	}
	return ""
}

// Has 判断是否存在同名头字段。
func (p *Part) Has(name string) bool {
	for _, f := range p.Fields {
		if strings.EqualFold(f.Name, name) {
			return true
		}
	}
	return false
}

// Set 设置头字段：替换第一个同名字段（保持位置）并删除其余同名字段；不存在时追加到末尾。
// value 不得含 CR/LF（防 header injection）。
func (p *Part) Set(name, value string) error {
	if strings.ContainsAny(name, "\r\n:") || strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("头字段 %s 的值不能含 CR/LF（防 header injection）", name)
	}
	raw := name + ": " + value
	replaced := false
	out := p.Fields[:0]
	for _, f := range p.Fields {
		if strings.EqualFold(f.Name, name) {
			if replaced {
				continue
			}
			f = Field{Name: name, Raw: raw}
			replaced = true
		}
		out = append(out, f)
	}
	p.Fields = out
	if !replaced {
		p.Fields = append(p.Fields, Field{Name: name, Raw: raw})
	}
	return nil
}

// Del 删除所有同名头字段。
func (p *Part) Del(name string) {
	out := p.Fields[:0]
	for _, f := range p.Fields {
		if !strings.EqualFold(f.Name, name) {
			out = append(out, f)
		}
	}
	p.Fields = out
}

// MediaType 返回小写的媒体类型与参数；缺省为 text/plain。
func (p *Part) MediaType() (string, map[string]string) {
	ct := p.Get("Content-Type")
	if ct == "" {
		return "text/plain", map[string]string{}
	}
	mt, params, err := mime.ParseMediaType(ct)
	if err != nil {
		// 容错：取分号前的部分
		mt = strings.ToLower(strings.TrimSpace(strings.SplitN(ct, ";", 2)[0]))
		params = map[string]string{}
	}
	return strings.ToLower(mt), params
}

// IsAttachment 判断是否为附件（Content-Disposition: attachment）。
func (p *Part) IsAttachment() bool {
	cd := p.Get("Content-Disposition")
	if cd == "" {
		return false
	}
	disp, _, err := mime.ParseMediaType(cd)
	if err != nil {
		disp = strings.ToLower(strings.TrimSpace(strings.SplitN(cd, ";", 2)[0]))
	}
	return strings.EqualFold(disp, "attachment")
}

// Decoded 按 Content-Transfer-Encoding 解码叶子 part 的内容（不做字符集转换）。
func (p *Part) Decoded() ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(p.Get("Content-Transfer-Encoding"))) {
	case "base64":
		clean := strings.Map(func(r rune) rune {
			switch r {
			case '\r', '\n', ' ', '\t':
				return -1
			}
			return r
		}, string(p.Body))
		if b, err := base64.StdEncoding.DecodeString(clean); err == nil {
			return b, nil
		}
		return base64.RawStdEncoding.DecodeString(strings.TrimRight(clean, "="))
	case "quoted-printable":
		return io.ReadAll(quotedprintable.NewReader(bytes.NewReader(p.Body)))
	default:
		return p.Body, nil
	}
}

// SetContent 用新内容替换叶子 part：Content-Type 设为 mediaType（UTF-8），传输编码统一为 base64。
// 其余头（Content-Disposition、Content-ID 等）保留。
func (p *Part) SetContent(mediaType string, data []byte) error {
	if p.Boundary != "" {
		return fmt.Errorf("不能直接替换 multipart 实体的内容")
	}
	if err := p.Set("Content-Type", mediaType+`; charset="UTF-8"`); err != nil {
		return err
	}
	if err := p.Set("Content-Transfer-Encoding", "base64"); err != nil {
		return err
	}
	eol := p.eol
	if eol == "" {
		eol = "\r\n"
	}
	enc := base64.StdEncoding.EncodeToString(data)
	var b strings.Builder
	for i := 0; i < len(enc); i += 76 {
		end := i + 76
		if end > len(enc) {
			end = len(enc)
		}
		b.WriteString(enc[i:end])
		b.WriteString(eol)
	}
	p.Body = []byte(b.String())
	return nil
}

// Walk 深度优先遍历所有 part（含自身）。
func (p *Part) Walk(fn func(*Part)) {
	fn(p)
	for _, c := range p.Children {
		c.Walk(fn)
	}
}

// RemoveChild 从子树中移除 target（按指针匹配），返回是否找到。
func (p *Part) RemoveChild(target *Part) bool {
	for i, c := range p.Children {
		if c == target {
			p.Children = append(p.Children[:i], p.Children[i+1:]...)
			return true
		}
		if c.RemoveChild(target) {
			return true
		}
	}
	return false
}

// BodyParts 找到主正文的 text/plain 与 text/html 叶子 part（跳过附件），各取第一个。
func (p *Part) BodyParts() (plain, html *Part) {
	p.Walk(func(part *Part) {
		if part.Boundary != "" || part.IsAttachment() {
			return
		}
		mt, _ := part.MediaType()
		switch mt {
		case "text/plain":
			if plain == nil {
				plain = part
			}
		case "text/html":
			if html == nil {
				html = part
			}
		}
	})
	return plain, html
}
