package xlsx

import (
	"fmt"
	"io/ioutil"
	"strings"
)

// File is a minimal in-repo replacement for the tealeg/xlsx File type. It stores
// sheet data in-memory and emits a simple text representation when saved. While
// not a real XLSX writer, it preserves the existing API surface used by the
// simulations so they can be built and exercised offline.
type File struct {
	Sheets []*Sheet
}

// Sheet holds a matrix of cell values.
type Sheet struct {
	Name string
	Rows []*Row
}

// Row is a single row within a sheet.
type Row struct {
	sheet *Sheet
	cells []*Cell
}

// Cell represents a single cell in a row.
type Cell struct {
	row   *Row
	Value string
}

// NewFile constructs an empty workbook.
func NewFile() *File {
	return &File{}
}

// AddSheet appends a new sheet to the workbook.
func (f *File) AddSheet(name string) (*Sheet, error) {
	sheet := &Sheet{Name: name}
	f.Sheets = append(f.Sheets, sheet)
	return sheet, nil
}

// AddRow appends a new row to the sheet and returns it.
func (s *Sheet) AddRow() *Row {
	row := &Row{sheet: s}
	s.Rows = append(s.Rows, row)
	return row
}

// AddCell appends a new cell to the row.
func (r *Row) AddCell() *Cell {
	cell := &Cell{row: r}
	r.cells = append(r.cells, cell)
	return cell
}

// Save writes a simple textual representation of the workbook. Each sheet is
// prefixed by its name and rows are comma separated, which is sufficient for
// quick inspection and keeps dependencies light for the simulation tooling.
func (f *File) Save(filename string) error {
	var builder strings.Builder

	for sheetIdx, sheet := range f.Sheets {
		if sheetIdx > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(fmt.Sprintf("Sheet: %s\n", sheet.Name))
		for _, row := range sheet.Rows {
			values := make([]string, len(row.cells))
			for i, cell := range row.cells {
				values[i] = escapeCell(cell.Value)
			}
			builder.WriteString(strings.Join(values, ","))
			builder.WriteString("\n")
		}
	}

	return ioutil.WriteFile(filename, []byte(builder.String()), 0644)
}

func escapeCell(value string) string {
	if strings.ContainsAny(value, ",\n\r") {
		return fmt.Sprintf("\"%s\"", strings.ReplaceAll(value, "\"", "\"\""))
	}
	return value
}
