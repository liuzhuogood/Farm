package automation

import (
	"math"
	"strings"
)

// centerDistance 计算两个中心坐标之间的欧氏距离
func centerDistance(x1, y1, x2, y2 int) float64 {
	dx := float64(x1 - x2)
	dy := float64(y1 - y2)
	return math.Sqrt(dx*dx + dy*dy)
}

// elementsEqual 判定两个元素的核心显示属性与状态是否完全一致
func elementsEqual(a, b *Element) bool {
	return a.Role == b.Role &&
		a.Label == b.Label &&
		a.CenterX == b.CenterX &&
		a.CenterY == b.CenterY &&
		a.Focused == b.Focused &&
		a.Editable == b.Editable &&
		a.Checked == b.Checked &&
		a.Disabled == b.Disabled &&
		a.Selected == b.Selected &&
		a.Scrollable == b.Scrollable &&
		a.Password == b.Password
}

// Diff 对比前序屏幕与当前屏幕，优先生成增量差异；若不满足增量条件则自动降级返回全量文本
// 返回值：(diff 或全量文本, 是否为 diff 增量)
func Diff(prev, next *Screen, formats []string) (string, bool) {
	if next == nil {
		return "", false
	}

	includeRef, includeOCR := parseFormats(formats)

	// 1. 回退判定：前序快照为空
	if prev == nil {
		return next.Render(formats), false
	}

	// 2. 回退判定：前台应用切换
	if prev.PackageName != "" && next.PackageName != "" && prev.PackageName != next.PackageName {
		return next.Render(formats), false
	}

	// 3. 回退判定：屏幕尺寸或方向旋转
	if prev.Width != next.Width || prev.Height != next.Height {
		return next.Render(formats), false
	}
	if prev.Orientation != "" && next.Orientation != "" && prev.Orientation != next.Orientation {
		return next.Render(formats), false
	}

	// 4. 对比 Elements
	var elemAdded []*Element
	var elemRemoved []*Element
	var elemModified []*Element

	if includeRef {
		prevUsed := make([]bool, len(prev.Elements))
		nextMatched := make([]int, len(next.Elements))
		for i := range nextMatched {
			nextMatched[i] = -1
		}

		// 匹配轮次 1：ResourceID + Role 精确匹配 (距离 <= 120)
		for ni, ne := range next.Elements {
			if ne.ResourceID == "" {
				continue
			}
			bestPi := -1
			bestDist := 120.0
			for pi, pe := range prev.Elements {
				if prevUsed[pi] || pe.ResourceID != ne.ResourceID || pe.Role != ne.Role {
					continue
				}
				dist := centerDistance(ne.CenterX, ne.CenterY, pe.CenterX, pe.CenterY)
				if dist <= bestDist {
					bestDist = dist
					bestPi = pi
				}
			}
			if bestPi >= 0 {
				nextMatched[ni] = bestPi
				prevUsed[bestPi] = true
			}
		}

		// 匹配轮次 2：完全相同的 Bounds + Role
		for ni, ne := range next.Elements {
			if nextMatched[ni] >= 0 {
				continue
			}
			for pi, pe := range prev.Elements {
				if prevUsed[pi] || pe.Role != ne.Role {
					continue
				}
				if pe.Bounds == ne.Bounds {
					nextMatched[ni] = pi
					prevUsed[pi] = true
					break
				}
			}
		}

		// 匹配轮次 3：相同 Role + Label + 临近距离 (<= 60)
		for ni, ne := range next.Elements {
			if nextMatched[ni] >= 0 {
				continue
			}
			bestPi := -1
			bestDist := 60.0
			for pi, pe := range prev.Elements {
				if prevUsed[pi] || pe.Role != ne.Role || pe.Label != ne.Label {
					continue
				}
				dist := centerDistance(ne.CenterX, ne.CenterY, pe.CenterX, pe.CenterY)
				if dist <= bestDist {
					bestDist = dist
					bestPi = pi
				}
			}
			if bestPi >= 0 {
				nextMatched[ni] = bestPi
				prevUsed[bestPi] = true
			}
		}

		// 统计变化与未匹配项
		for ni, ne := range next.Elements {
			pi := nextMatched[ni]
			if pi < 0 {
				elemAdded = append(elemAdded, &next.Elements[ni])
			} else {
				pe := &prev.Elements[pi]
				if !elementsEqual(pe, &ne) {
					elemModified = append(elemModified, &next.Elements[ni])
				}
			}
		}

		for pi, pe := range prev.Elements {
			if !prevUsed[pi] {
				elemRemoved = append(elemRemoved, &pe)
			}
		}
	}

	// 5. 对比 OCR
	var ocrAdded []*OCRItem
	var ocrRemoved []*OCRItem

	if includeOCR {
		prevOCRUsed := make([]bool, len(prev.OCR))
		for _, no := range next.OCR {
			matched := false
			bestPi := -1
			bestDist := 40.0
			for pi, po := range prev.OCR {
				if prevOCRUsed[pi] || po.Text != no.Text {
					continue
				}
				dist := centerDistance(no.CenterX, no.CenterY, po.CenterX, po.CenterY)
				if dist <= bestDist {
					bestDist = dist
					bestPi = pi
					matched = true
				}
			}
			if matched && bestPi >= 0 {
				prevOCRUsed[bestPi] = true
			} else {
				item := no
				ocrAdded = append(ocrAdded, &item)
			}
		}

		for pi, po := range prev.OCR {
			if !prevOCRUsed[pi] {
				item := po
				ocrRemoved = append(ocrRemoved, &item)
			}
		}
	}

	// 6. 回退判定：变更行数超过全屏
	totalChanges := len(elemAdded) + len(elemRemoved) + len(elemModified) + len(ocrAdded) + len(ocrRemoved)
	totalCapacity := 0
	if includeRef {
		totalCapacity += len(next.Elements)
	}
	if includeOCR {
		totalCapacity += len(next.OCR)
	}

	if totalCapacity > 0 && totalChanges >= totalCapacity {
		return next.Render(formats), false
	}

	// 7. 组装 Diff 输出
	var lines []string
	lines = append(lines, next.MetadataLine())

	for _, ev := range next.Events {
		lines = append(lines, ev.Line())
	}

	// 移除的元素
	for _, pe := range elemRemoved {
		lines = append(lines, "- "+pe.Line())
	}

	// 新增的元素
	for _, ne := range elemAdded {
		lines = append(lines, "+ "+ne.Line())
	}

	// 属性发生变化的元素
	for _, me := range elemModified {
		lines = append(lines, "~ "+me.Line())
	}

	// 移除的 OCR
	for _, po := range ocrRemoved {
		lines = append(lines, "- "+po.Line())
	}

	// 新增的 OCR
	for _, no := range ocrAdded {
		lines = append(lines, "+ "+no.Line())
	}

	return strings.Join(lines, "\n"), true
}
