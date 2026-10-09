// 部分实现改编自 larksuite/cli（MIT License, Copyright (c) 2026 Lark Technologies Pte. Ltd.）
// SPDX-License-Identifier: MIT

// Package docxparse 把飞书 docs_ai DocxXML 解析为轻量 DOM，供 `doc script --command parse`
// 统计字数 / 块分布并做容错修复（对齐官方 lark-cli docs +script 的 docxparse 包）。
//
// 只做 XML 语法与画像，不校验业务 schema；能否写入以服务端结果为准。
package docxparse

// Format is an accepted source document format.
type Format string

const (
	FormatXML Format = "xml"
)

// ParseResult is the complete result returned by Parse.
type ParseResult struct {
	Format  Format  `json:"format"`
	XML     string  `json:"xml"`
	Profile Profile `json:"profile"`
}

type nodeType uint8

const (
	nodeText nodeType = iota
	nodeElement
)

// Node is the internal DocxXML DOM representation.
type Node struct {
	typ      nodeType
	tag      string
	attrs    map[string]string
	children []*Node
	text     string
	parent   *Node
}

func newText(text string) *Node {
	return &Node{typ: nodeText, text: text}
}

func newElement(tag string, attrs map[string]string) *Node {
	return &Node{typ: nodeElement, tag: tag, attrs: attrs}
}

func (n *Node) addChild(child *Node) {
	if n == nil || child == nil {
		return
	}
	child.parent = n
	n.children = append(n.children, child)
}
