package cmd

import (
	"fmt"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/converter"
)

// fillTableWithExtraRows 幂等地填充飞书表格：
//  1. 根据当前实际行数计算还缺多少行（重试场景不会重复追加，避免行数倍增）
//  2. 通过 insert_table_row API 追加缺失的扩展行
//  3. 填充所有单元格（FillTableCells 对同一 cell 写同值是幂等的）
//
// 被 doc import（processTableTask）和 doc add/content-update（fillTableWithRetry）共用。
// onAppendProgress 可为 nil；由调用方决定如何展示（syncPrintf / fmt.Printf）。
//
// cellMap 为可选的 cellID -> textBlockID 映射（import 阶段二开头一次性
// GetAllBlocksWithToken 构建，三个 table worker 共享，只读）。cellMap 为 nil
// （doc add / content-update 路径）或本次发生追加行时，函数内部按本表格局部
// 拉取一次直接子块补建映射，保证 batch_update 快路径在两条路径上都生效；
// 补建失败自动降级为逐 cell 旧路径，行为不劣于改造前。
func fillTableWithExtraRows(
	documentID, tableBlockID string,
	td *converter.TableData,
	userAccessToken string,
	onAppendProgress client.InsertRowProgressFunc,
	cellMap map[string]string,
) error {
	if td.Cols <= 0 {
		return fmt.Errorf("表格列数 Cols=%d 不合法", td.Cols)
	}

	cellIDs, err := client.GetTableCellIDs(documentID, tableBlockID, userAccessToken)
	if err != nil {
		return fmt.Errorf("获取单元格失败: %w", err)
	}

	currentRows := len(cellIDs) / td.Cols
	targetRows := td.Rows + len(td.ExtraRowContents)

	// 幂等追加：仅追加"缺失"的行。若重试时发现已追加过，就不会再加。
	appended := false
	if currentRows < targetRows {
		missing := targetRows - currentRows
		if err := client.AppendTableRows(documentID, tableBlockID, missing, onAppendProgress, userAccessToken); err != nil {
			return err
		}
		appended = true
		cellIDs, err = client.GetTableCellIDs(documentID, tableBlockID, userAccessToken)
		if err != nil {
			return fmt.Errorf("获取追加后单元格失败: %w", err)
		}
	}

	// 补建 cellID -> textBlockID 映射（issue #172）：
	// - cellMap 为 nil：doc add / content-update 全部 mode 此前恒传 nil，导致所有 cell
	//   走逐 cell 慢路径，batch_update 优化从未生效；
	// - 发生追加行：新行 cell 不在 import 预热的全文档映射里，此前同样逐 cell 降级。
	// 两种情况都按本表格局部拉取一次直接子块（cell 块自带 Children，读接口不占
	// 3 QPS 写限流），成本与表格规模成正比，避免对存量大文档做全文档拉取。
	// 该逻辑位于重试闭包内，重试时自动重建，无映射过期问题。
	// 构建失败不致命：FillTableCellsRichWithMap 对缺映射的 cell 自动降级。
	if cellMap == nil || appended {
		if local, buildErr := buildTableCellMap(documentID, tableBlockID, userAccessToken); buildErr == nil {
			// 不改写传入的 cellMap：import 的多个 table worker 共享同一只读 map
			cellMap = mergeCellMaps(cellMap, local)
		}
	}

	// 填充初始单元格（幂等）
	initialCellCount := td.Rows * td.Cols
	if initialCellCount > len(cellIDs) {
		initialCellCount = len(cellIDs)
	}
	initialCellIDs := cellIDs[:initialCellCount]
	if len(td.CellElements) > 0 {
		if err := client.FillTableCellsRichWithMap(documentID, initialCellIDs, td.CellElements, td.CellContents, cellMap, userAccessToken); err != nil {
			return fmt.Errorf("填充初始内容失败: %w", err)
		}
	} else if err := client.FillTableCells(documentID, initialCellIDs, td.CellContents, userAccessToken); err != nil {
		return fmt.Errorf("填充初始内容失败: %w", err)
	}

	if len(td.ExtraRowContents) == 0 {
		return nil
	}

	// 扁平化扩展行内容 + 填充。追加行的新 cell 已由上方局部补建进 cellMap，
	// 与初始 cell 一样享受 batch_update；个别缺映射的 cell 自动降级到逐 cell 路径。
	extraContents, extraElements := flattenExtraRows(td)
	newCellIDs := cellIDs[initialCellCount:]
	if len(newCellIDs) < len(extraContents) {
		return fmt.Errorf("扩展行单元格不足: 实际 %d, 需要 %d", len(newCellIDs), len(extraContents))
	}
	newCellIDs = newCellIDs[:len(extraContents)]

	if len(extraElements) > 0 {
		if err := client.FillTableCellsRichWithMap(documentID, newCellIDs, extraElements, extraContents, cellMap, userAccessToken); err != nil {
			return fmt.Errorf("填充扩展行失败: %w", err)
		}
		return nil
	}
	if err := client.FillTableCells(documentID, newCellIDs, extraContents, userAccessToken); err != nil {
		return fmt.Errorf("填充扩展行失败: %w", err)
	}
	return nil
}

// buildTableCellMap 按单个表格局部拉取直接子块（cell 块，响应自带 Children 字段），
// 构建 cellID -> 默认空 text 子块 ID 的映射。与全文档 buildCellTextBlockMap 相比，
// 读成本按表格自身规模计费（≤500 cell 单次分页调用），适合 content-update 面向
// 存量大文档、只新增少量表格的场景。
func buildTableCellMap(documentID, tableBlockID, userAccessToken string) (map[string]string, error) {
	blocks, err := client.GetAllBlockChildren(documentID, tableBlockID, userAccessToken)
	if err != nil {
		return nil, err
	}
	return cellTextBlockMapFromBlocks(blocks), nil
}

// cellTextBlockMapFromBlocks 从块列表筛出 TableCell（BlockType 32）且有子块的项，
// 构建 cellID -> Children[0]（cell 的默认空 text 块 ID）映射。
// 纯函数，供全文档（buildCellTextBlockMap）与单表格（buildTableCellMap）两条构建路径共用。
func cellTextBlockMapFromBlocks(blocks []*larkdocx.Block) map[string]string {
	cellMap := make(map[string]string, len(blocks)/4) // cell 占比经验值
	for _, b := range blocks {
		if b == nil || b.BlockType == nil || *b.BlockType != int(converter.BlockTypeTableCell) {
			continue
		}
		if len(b.Children) == 0 {
			continue
		}
		cellID := client.StringVal(b.BlockId)
		if cellID == "" {
			continue
		}
		cellMap[cellID] = b.Children[0]
	}
	return cellMap
}

// mergeCellMaps 合并共享映射与局部映射（局部结果更新鲜、优先），不改写任一输入。
// shared 可为 nil。
func mergeCellMaps(shared, local map[string]string) map[string]string {
	merged := make(map[string]string, len(shared)+len(local))
	for k, v := range shared {
		merged[k] = v
	}
	for k, v := range local {
		merged[k] = v
	}
	return merged
}

// flattenExtraRows 将 TableData.ExtraRow{Contents,Elements} 从二维扁平化为 cell 数组。
// 返回的 elements 为 nil 时表示无富文本数据，调用方应使用纯文本填充路径。
func flattenExtraRows(td *converter.TableData) ([]string, [][]*larkdocx.TextElement) {
	n := len(td.ExtraRowContents)
	if n == 0 {
		return nil, nil
	}
	useRich := len(td.ExtraRowElements) > 0
	contents := make([]string, 0, n*td.Cols)
	var elements [][]*larkdocx.TextElement
	if useRich {
		elements = make([][]*larkdocx.TextElement, 0, n*td.Cols)
	}
	for i, row := range td.ExtraRowContents {
		contents = append(contents, row...)
		if useRich && i < len(td.ExtraRowElements) {
			elements = append(elements, td.ExtraRowElements[i]...)
		}
	}
	return contents, elements
}

// tableAppendProgress 返回用于 AppendTableRows 的进度回调；
// 当扩展行数 >= threshold 且 logger 非空时每 step 行打印一次，在最后一行也打印。
// logger("追加进度 %d/%d")。若条件不满足返回 nil（无回调）。
func tableAppendProgress(extraRowCount, threshold, step int, logger func(appended, total int)) client.InsertRowProgressFunc {
	if logger == nil || extraRowCount < threshold || step < 1 {
		return nil
	}
	return func(appended, total int) {
		if appended == total || appended%step == 0 {
			logger(appended, total)
		}
	}
}
