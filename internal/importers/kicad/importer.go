package kicad

import (
	"encoding/csv"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/badimirzai/robotics-verifier-cli/internal/ir"
)

var defaultSynonyms = map[string][]string{
	"ref":          {"Ref", "Reference", "Designator", "References", "Reference(s)", "Designators", "RefDes"},
	"value":        {"Value", "Component", "Comment", "Designation", "Description", "Desc"},
	"footprint":    {"Footprint", "Footprints", "Package", "PCB Footprint"},
	"mpn":          {"MPN", "Manufacturer Part Number", "Part Number"},
	"manufacturer": {"Manufacturer", "Mfr"},
	"quantity":     {"Qty", "Quantity"},
}

var supportedDelimiters = []rune{',', ';', '\t'}

type resolvedColumns struct {
	ref          int
	value        int
	footprint    int
	mpn          int
	manufacturer int
	quantity     int
}

// ImportKiCadBOM imports a KiCad BOM CSV file into a stable DesignIR model.
// Row-level parse issues are recorded on the returned DesignIR instead of
// aborting the entire import so the scan command can always emit a report.
func ImportKiCadBOM(path string, mapping ColumnMapping) (*ir.DesignIR, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("open BOM CSV: %w", err)
	}

	design := &ir.DesignIR{
		Source: "kicad_bom_csv",
		Metadata: ir.IRMetadata{
			InputFile: path,
			ParsedAt:  time.Now().UTC().Format(time.RFC3339),
		},
	}

	lines := splitNormalizedLines(string(content))
	delimiter := detectDelimiter(lines)

	headerLine, headers, columns, err := findHeaderRow(lines, delimiter, mapping)
	if err != nil {
		design.ParseErrors = append(design.ParseErrors, err.Error())
		return design, nil
	}

	uniqueFieldHeaders := uniqueHeaders(headers)
	parts := make([]ir.Part, 0, 64)
	parseErrors := make([]string, 0)
	parseWarnings := make([]string, 0)

	for idx := headerLine; idx < len(lines); idx++ {
		lineNumber := idx + 1
		line := lines[idx]
		if strings.TrimSpace(line) == "" {
			continue
		}

		record, recordErr := parseDelimitedLine(line, delimiter)
		if recordErr != nil {
			parseErrors = append(parseErrors, fmt.Sprintf("row %d: malformed CSV row: %v", lineNumber, recordErr))
			continue
		}
		if isBlankRecord(record) {
			continue
		}

		rowWarnings, rowErrors, rowParts := buildPartsFromRecord(record, uniqueFieldHeaders, headers, columns, lineNumber)
		parseWarnings = append(parseWarnings, rowWarnings...)
		parseErrors = append(parseErrors, rowErrors...)
		parts = append(parts, rowParts...)
	}

	design.Parts = parts
	design.ParseErrors = parseErrors
	design.ParseWarnings = parseWarnings
	return design, nil
}

func buildPartsFromRecord(record []string, uniqueFieldHeaders []string, headers []string, columns resolvedColumns, row int) ([]string, []string, []ir.Part) {
	parseWarnings := make([]string, 0, 1)
	parseErrors := make([]string, 0, 2)

	// Short rows are treated as hard parse errors and skipped because missing
	// delimiters make the remaining column alignment ambiguous.
	if len(record) < len(headers) {
		parseErrors = append(parseErrors, fmt.Sprintf("row %d: malformed CSV row: expected %d columns from header, got %d", row, len(headers), len(record)))
		return parseWarnings, parseErrors, nil
	}

	refValue := cell(record, columns.ref)
	value := cell(record, columns.value)
	footprint := cell(record, columns.footprint)
	fields := rawFields(uniqueFieldHeaders, record)

	if refValue == "" {
		if qty := cell(record, columns.quantity); qty != "" {
			parseErrors = append(parseErrors, fmt.Sprintf("row %d: quantity present (%s=%s) but ref is empty; explicit refs are required for now", row, columnName(headers, columns.quantity), qty))
		} else {
			parseErrors = append(parseErrors, missingFieldMessage(row, refValue, "required field ref", columnName(headers, columns.ref)))
		}
		return parseWarnings, parseErrors, nil
	}
	if value == "" {
		parseErrors = append(parseErrors, missingFieldMessage(row, refValue, "required field value", columnName(headers, columns.value)))
		return parseWarnings, parseErrors, nil
	}
	if footprint == "" {
		parseWarnings = append(parseWarnings, missingFieldMessage(row, refValue, "recommended field footprint", columnName(headers, columns.footprint)))
	}

	refs := splitReferences(refValue)
	if len(refs) == 0 {
		parseErrors = append(parseErrors, missingFieldMessage(row, refValue, "required field ref", columnName(headers, columns.ref)))
		return parseWarnings, parseErrors, nil
	}

	template := ir.Part{
		Value:     value,
		Footprint: footprint,
		Fields:    fields,
	}
	if mpn := cell(record, columns.mpn); mpn != "" {
		template.MPN = mpn
	}
	if manufacturer := cell(record, columns.manufacturer); manufacturer != "" {
		template.Manufacturer = manufacturer
	}

	parts := make([]ir.Part, 0, len(refs))
	for _, ref := range refs {
		part := template
		part.Ref = ref
		part.Fields = cloneFields(fields)
		parts = append(parts, part)
	}

	return parseWarnings, parseErrors, parts
}

func findHeaderRow(lines []string, delimiter rune, mapping ColumnMapping) (int, []string, resolvedColumns, error) {
	lastErr := error(nil)
	bestScore := -1
	for idx, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		record, err := parseDelimitedLine(line, delimiter)
		if err != nil {
			lastErr = fmt.Errorf("row %d: malformed CSV header row: %v", idx+1, err)
			continue
		}
		if len(record) < 2 || isBlankRecord(record) {
			continue
		}

		headers := sanitizeHeaders(record)
		columns, resolveErr := resolveColumns(headers, mapping)
		if resolveErr == nil {
			return idx + 1, headers, columns, nil
		}
		score := headerCandidateScore(headers, mapping)
		if score > bestScore {
			bestScore = score
			lastErr = fmt.Errorf("row %d: %w", idx+1, resolveErr)
		}
	}

	if lastErr != nil {
		return 0, nil, resolvedColumns{}, lastErr
	}
	return 0, nil, resolvedColumns{}, fmt.Errorf("no BOM header row found")
}

func headerCandidateScore(headers []string, mapping ColumnMapping) int {
	score := 0
	if headerMatches(headers, "ref", mapping.Ref) {
		score++
	}
	if headerMatches(headers, "value", mapping.Value) {
		score++
	}
	if headerMatches(headers, "footprint", mapping.Footprint) {
		score++
	}
	if headerMatches(headers, "mpn", mapping.MPN) {
		score++
	}
	if headerMatches(headers, "manufacturer", mapping.Manufacturer) {
		score++
	}
	if headerMatches(headers, "quantity", "") {
		score++
	}
	return score
}

func headerMatches(headers []string, fieldName string, mappedColumn string) bool {
	if strings.TrimSpace(mappedColumn) != "" {
		return indexForHeader(headers, mappedColumn) != -1
	}
	return autoDetectHeader(headers, defaultSynonyms[fieldName]) != -1
}

func detectDelimiter(lines []string) rune {
	samples := firstNonEmptyLines(lines, 5)
	bestDelimiter := supportedDelimiters[0]
	bestScore := delimiterScore{}

	for order, delimiter := range supportedDelimiters {
		score := scoreDelimiter(samples, delimiter)
		score.order = order
		if betterDelimiterScore(score, bestScore) {
			bestDelimiter = delimiter
			bestScore = score
		}
	}

	return bestDelimiter
}

type delimiterScore struct {
	multiFieldMatches int
	multiFieldColumns int
	overallMatches    int
	overallColumns    int
	order             int
}

func scoreDelimiter(lines []string, delimiter rune) delimiterScore {
	counts := make([]int, 0, len(lines))
	for _, line := range lines {
		record, err := parseDelimitedLine(line, delimiter)
		if err != nil {
			counts = append(counts, 0)
			continue
		}
		counts = append(counts, len(record))
	}

	score := delimiterScore{}
	score.multiFieldMatches, score.multiFieldColumns = mostFrequentCount(counts, true)
	score.overallMatches, score.overallColumns = mostFrequentCount(counts, false)
	return score
}

func betterDelimiterScore(left delimiterScore, right delimiterScore) bool {
	if left.multiFieldMatches != right.multiFieldMatches {
		return left.multiFieldMatches > right.multiFieldMatches
	}
	if left.multiFieldColumns != right.multiFieldColumns {
		return left.multiFieldColumns > right.multiFieldColumns
	}
	if left.overallMatches != right.overallMatches {
		return left.overallMatches > right.overallMatches
	}
	if left.overallColumns != right.overallColumns {
		return left.overallColumns > right.overallColumns
	}
	return left.order < right.order
}

func mostFrequentCount(counts []int, multiFieldOnly bool) (int, int) {
	if len(counts) == 0 {
		return 0, 0
	}

	freq := make(map[int]int, len(counts))
	bestMatches := 0
	bestColumns := 0
	for _, count := range counts {
		if multiFieldOnly && count <= 1 {
			continue
		}
		freq[count]++
		if freq[count] > bestMatches || (freq[count] == bestMatches && count > bestColumns) {
			bestMatches = freq[count]
			bestColumns = count
		}
	}
	return bestMatches, bestColumns
}

func firstNonEmptyLines(lines []string, limit int) []string {
	out := make([]string, 0, limit)
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		out = append(out, line)
		if len(out) == limit {
			break
		}
	}
	return out
}

func parseDelimitedLine(line string, delimiter rune) ([]string, error) {
	reader := csv.NewReader(strings.NewReader(line))
	reader.Comma = delimiter
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true
	return reader.Read()
}

func splitNormalizedLines(content string) []string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	return strings.Split(content, "\n")
}

func resolveColumns(headers []string, mapping ColumnMapping) (resolvedColumns, error) {
	columns := resolvedColumns{
		ref:          -1,
		value:        -1,
		footprint:    -1,
		mpn:          -1,
		manufacturer: -1,
		quantity:     -1,
	}

	var err error
	columns.ref, err = resolveColumn(headers, "ref", mapping.Ref, true)
	if err != nil {
		return columns, err
	}
	columns.value, err = resolveColumn(headers, "value", mapping.Value, true)
	if err != nil {
		return columns, err
	}
	columns.footprint, err = resolveColumn(headers, "footprint", mapping.Footprint, true)
	if err != nil {
		return columns, err
	}
	columns.mpn, err = resolveColumn(headers, "mpn", mapping.MPN, false)
	if err != nil {
		return columns, err
	}
	columns.manufacturer, err = resolveColumn(headers, "manufacturer", mapping.Manufacturer, false)
	if err != nil {
		return columns, err
	}
	columns.quantity, err = resolveColumn(headers, "quantity", "", false)
	if err != nil {
		return columns, err
	}

	return columns, nil
}

func resolveColumn(headers []string, fieldName string, mappedColumn string, required bool) (int, error) {
	if strings.TrimSpace(mappedColumn) != "" {
		idx := indexForHeader(headers, mappedColumn)
		if idx == -1 {
			return -1, fmt.Errorf("mapped %s column %q not found in BOM headers", fieldName, mappedColumn)
		}
		return idx, nil
	}

	idx := autoDetectHeader(headers, defaultSynonyms[fieldName])
	if idx == -1 && required {
		return -1, fmt.Errorf("missing required BOM column for %q (expected one of: %s)", fieldName, strings.Join(defaultSynonyms[fieldName], ", "))
	}
	return idx, nil
}

func autoDetectHeader(headers []string, candidates []string) int {
	for _, candidate := range candidates {
		idx := indexForHeader(headers, candidate)
		if idx != -1 {
			return idx
		}
	}
	return -1
}

func indexForHeader(headers []string, header string) int {
	target := normalizeHeader(header)
	for i, h := range headers {
		if normalizeHeader(h) == target {
			return i
		}
	}
	return -1
}

func sanitizeHeaders(headers []string) []string {
	sanitized := make([]string, len(headers))
	for i, header := range headers {
		sanitized[i] = strings.TrimSpace(strings.TrimPrefix(header, "\ufeff"))
	}
	return sanitized
}

func normalizeHeader(s string) string {
	s = strings.TrimSpace(strings.TrimPrefix(s, "\ufeff"))
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func uniqueHeaders(headers []string) []string {
	seen := make(map[string]int, len(headers))
	unique := make([]string, len(headers))
	for i, h := range headers {
		name := strings.TrimSpace(h)
		if name == "" {
			name = fmt.Sprintf("column_%d", i+1)
		}
		count := seen[name]
		if count == 0 {
			unique[i] = name
		} else {
			unique[i] = fmt.Sprintf("%s_%d", name, count+1)
		}
		seen[name] = count + 1
	}
	return unique
}

func rawFields(headers []string, record []string) map[string]string {
	fields := make(map[string]string, len(headers))
	for i, header := range headers {
		fields[header] = cell(record, i)
	}
	return fields
}

func cloneFields(fields map[string]string) map[string]string {
	if len(fields) == 0 {
		return nil
	}
	out := make(map[string]string, len(fields))
	for key, value := range fields {
		out[key] = value
	}
	return out
}

func missingFieldMessage(row int, ref string, field string, column string) string {
	if ref != "" {
		return fmt.Sprintf("row %d (ref=%s): missing %s (column=%s)", row, ref, field, column)
	}
	return fmt.Sprintf("row %d: missing %s (column=%s)", row, field, column)
}

func columnName(headers []string, idx int) string {
	if idx >= 0 && idx < len(headers) {
		header := strings.TrimSpace(headers[idx])
		if header != "" {
			return header
		}
	}
	return fmt.Sprintf("column_%d", idx+1)
}

func splitReferences(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || unicode.IsSpace(r)
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return slices.Clip(out)
}

func cell(record []string, idx int) string {
	if idx < 0 || idx >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[idx])
}

func isBlankRecord(record []string) bool {
	for _, v := range record {
		if strings.TrimSpace(v) != "" {
			return false
		}
	}
	return true
}
