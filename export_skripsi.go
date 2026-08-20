package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/xuri/excelize/v2"
)

func ExportSkripsi(c *fiber.Ctx) error {
	startDateStr := c.Query("start_date")
	endDateStr := c.Query("end_date")

	if startDateStr == "" || endDateStr == "" {
		return c.Status(400).JSON(fiber.Map{"error": "start_date dan end_date diperlukan"})
	}

	startDate, err := time.Parse("2006-01-02", startDateStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format start_date salah"})
	}

	endDate, err := time.Parse("2006-01-02", endDateStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format end_date salah"})
	}

	f := excelize.NewFile()
	isFirstSheet := true
	hasData := false

	// Ambil semua barang dari database agar yang 0 qty tetap tampil berurutan
	type BarangItem struct {
		ID         int
		NamaBarang string
	}
	var allBarangs []BarangItem
	if err := DB.Raw("SELECT id, nama_barang FROM barangs ORDER BY id ASC").Scan(&allBarangs).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	var uniqueBarangs []string
	for _, b := range allBarangs {
		uniqueBarangs = append(uniqueBarangs, b.NamaBarang)
	}

	for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
		dayOfWeek := int(d.Weekday())
		if dayOfWeek == 0 { // Skip Sunday
			continue
		}

		siklusAktif := ""
		siklusCondition := ""
		switch dayOfWeek {
		case 1, 4:
			siklusAktif = "SiklusKamisSenin"
			siklusCondition = "'SiklusKamisSenin'"
		case 2, 5:
			siklusAktif = "SiklusJumatSelasa"
			siklusCondition = "'SiklusJumatSelasa', 'SiklusDua'"
		case 3, 6:
			siklusAktif = "SiklusSabtuRabu"
			siklusCondition = "'SiklusSabtuRabu'"
		}

		// Run query for this day
		tanggal := d.Format("2006-01-02")
		
		siklusFilter := fmt.Sprintf(`(
			(nota.is_harian_snapshot = true AND '%s' != '') OR 
			(nota.siklus_snapshot IN (%s) AND nota.is_harian_snapshot = false)
		)`, siklusAktif, siklusCondition)

		kirimDateExpr := `CAST(
			CASE 
				WHEN nota.siklus_snapshot = 'HARIAN' THEN nota.tanggal_kirim
				WHEN nota.siklus_snapshot = 'SiklusDua' THEN 
					CASE 
						WHEN EXTRACT(DOW FROM nota.tanggal_kirim) IN (1,2,3) THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '3 days'
						WHEN EXTRACT(DOW FROM nota.tanggal_kirim) IN (4,5,6) THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '7 days'
						ELSE nota.tanggal_kirim
					END
				WHEN nota.siklus_snapshot = 'SiklusKamisSenin' THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '3 days'
				WHEN nota.siklus_snapshot = 'SiklusJumatSelasa' THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '4 days'
				WHEN nota.siklus_snapshot = 'SiklusSabtuRabu' THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '5 days'
				ELSE nota.tanggal_kirim
			END AS DATE)`

		returDateExpr := `CAST(
			CASE 
				WHEN nota.siklus_snapshot = 'HARIAN' THEN 
					CASE EXTRACT(ISODOW FROM nota.tanggal_kirim)
						WHEN 1 THEN DATE_TRUNC('week', nota.tanggal_kirim) - INTERVAL '4 days'
						WHEN 2 THEN DATE_TRUNC('week', nota.tanggal_kirim) - INTERVAL '3 days'
						WHEN 3 THEN DATE_TRUNC('week', nota.tanggal_kirim) - INTERVAL '2 days'
						WHEN 4 THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '0 days'
						WHEN 5 THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '1 days'
						WHEN 6 THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '2 days'
						WHEN 7 THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '2 days'
						ELSE nota.tanggal_kirim
					END
				WHEN nota.siklus_snapshot = 'SiklusDua' THEN 
					CASE 
						WHEN EXTRACT(DOW FROM nota.tanggal_kirim) IN (1,2,3) THEN DATE_TRUNC('week', nota.tanggal_kirim) - INTERVAL '3 days'
						WHEN EXTRACT(DOW FROM nota.tanggal_kirim) IN (4,5,6) THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '1 days'
						ELSE nota.tanggal_kirim
					END
				WHEN nota.siklus_snapshot = 'SiklusKamisSenin' THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '7 days'
				WHEN nota.siklus_snapshot = 'SiklusJumatSelasa' THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '8 days'
				WHEN nota.siklus_snapshot = 'SiklusSabtuRabu' THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '9 days'
				ELSE nota.tanggal_kirim + INTERVAL '4 days'
			END AS DATE)`

		query := fmt.Sprintf(`
			SELECT 
				barangs.id as barang_id,
				barangs.nama_barang, 
				tokos.nama_toko,
				MAX(nota.siklus_snapshot) as siklus,
				bool_or(nota.is_harian_snapshot) as is_harian, 
				COALESCE(SUM(CASE WHEN %s = CAST(? AS DATE) THEN nota_details.banyak_kirim ELSE 0 END), 0) as qty_kirim, 
				COALESCE(SUM(CASE WHEN %s = CAST(? AS DATE) THEN nota_details.banyak_retur ELSE 0 END), 0) as qty_retur, 
				COALESCE(SUM(CASE WHEN %s = CAST(? AS DATE) THEN nota_details.harga_kirim ELSE 0 END), 0) as harga_kirim,
				COALESCE(SUM(CASE WHEN %s = CAST(? AS DATE) THEN nota_details.harga_retur ELSE 0 END), 0) as harga_retur
			FROM nota_details
			JOIN nota ON nota.id = nota_details.nota_id
			JOIN tokos ON tokos.id = nota.toko_id
			JOIN barangs ON barangs.id = nota_details.barang_id
			WHERE 
				%s 
			AND 
				nota.tanggal_kirim >= CAST(? AS DATE) - INTERVAL '30 days'
			AND nota.status != 'DIBATALKAN'
			GROUP BY barangs.id, barangs.nama_barang, tokos.nama_toko
		`, kirimDateExpr, returDateExpr, kirimDateExpr, returDateExpr, siklusFilter)

		var results []CatatanBesarResult
		err := DB.Raw(query, tanggal, tanggal, tanggal, tanggal, tanggal).Scan(&results).Error
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		if len(results) == 0 {
			continue // Skip creating sheet if no data
		}

		hasData = true
		sheetName := d.Format("01-02")
		
		if isFirstSheet {
			f.SetSheetName("Sheet1", sheetName)
			isFirstSheet = false
		} else {
			f.NewSheet(sheetName)
		}

		tokosMap := make(map[string]TokoMeta)
		cellData := make(map[string]CatatanBesarResult)
		totalsMap := make(map[string]struct {
			TotalKirim float64
			TotalRetur float64
		})

		for _, row := range results {
			if _, ok := tokosMap[row.NamaToko]; !ok {
				tokosMap[row.NamaToko] = TokoMeta{NamaToko: row.NamaToko, Siklus: row.Siklus}
			}
			cellKey := row.NamaToko + "-" + row.NamaBarang
			existing := cellData[cellKey]
			existing.QtyKirim += row.QtyKirim
			existing.QtyRetur += row.QtyRetur
			cellData[cellKey] = existing

			tTotals := totalsMap[row.NamaToko]
			tTotals.TotalKirim += row.HargaKirim
			tTotals.TotalRetur += row.HargaRetur
			totalsMap[row.NamaToko] = tTotals
		}

		var filteredTokos []TokoMeta
		for _, t := range tokosMap {
			filteredTokos = append(filteredTokos, t)
		}

		sort.Slice(filteredTokos, func(i, j int) bool {
			pA := getSiklusPriority(filteredTokos[i].Siklus)
			pB := getSiklusPriority(filteredTokos[j].Siklus)
			if pA != pB {
				return pA < pB
			}
			return strings.Compare(filteredTokos[i].NamaToko, filteredTokos[j].NamaToko) < 0
		})

		// Styles
		headerStyle, _ := f.NewStyle(&excelize.Style{
			Font: &excelize.Font{Bold: true},
			Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
			Border: []excelize.Border{
				{Type: "left", Color: "#000000", Style: 1},
				{Type: "top", Color: "#000000", Style: 1},
				{Type: "bottom", Color: "#000000", Style: 1},
				{Type: "right", Color: "#000000", Style: 1},
			},
		})

		cellBorder, _ := f.NewStyle(&excelize.Style{
			Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
			Border: []excelize.Border{
				{Type: "left", Color: "#000000", Style: 1},
				{Type: "top", Color: "#000000", Style: 1},
				{Type: "bottom", Color: "#000000", Style: 1},
				{Type: "right", Color: "#000000", Style: 1},
			},
		})
		
		leftBorder, _ := f.NewStyle(&excelize.Style{
			Alignment: &excelize.Alignment{Vertical: "center"},
			Border: []excelize.Border{
				{Type: "left", Color: "#000000", Style: 1},
				{Type: "top", Color: "#000000", Style: 1},
				{Type: "bottom", Color: "#000000", Style: 1},
				{Type: "right", Color: "#000000", Style: 1},
			},
		})

		currencyStyle, _ := f.NewStyle(&excelize.Style{
			NumFmt: 3, // #,##0
			Font:   &excelize.Font{Bold: true},
			Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
			Border: []excelize.Border{
				{Type: "left", Color: "#000000", Style: 1},
				{Type: "top", Color: "#000000", Style: 1},
				{Type: "bottom", Color: "#000000", Style: 1},
				{Type: "right", Color: "#000000", Style: 1},
			},
		})
        
        boldBorder, _ := f.NewStyle(&excelize.Style{
			Font:   &excelize.Font{Bold: true},
			Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
			Border: []excelize.Border{
				{Type: "left", Color: "#000000", Style: 1},
				{Type: "top", Color: "#000000", Style: 1},
				{Type: "bottom", Color: "#000000", Style: 1},
				{Type: "right", Color: "#000000", Style: 1},
			},
		})

		// Headers
		f.SetCellValue(sheetName, "A1", "No")
		f.MergeCell(sheetName, "A1", "A2")
		f.SetCellStyle(sheetName, "A1", "A2", headerStyle)
		f.SetColWidth(sheetName, "A", "A", 5)

		f.SetCellValue(sheetName, "B1", "Produk")
		f.MergeCell(sheetName, "B1", "B2")
		f.SetCellStyle(sheetName, "B1", "B2", headerStyle)
		f.SetColWidth(sheetName, "B", "B", 25)

		colIndex := 3 // C
		for _, toko := range filteredTokos {
			colName, _ := excelize.ColumnNumberToName(colIndex)
			nextColName, _ := excelize.ColumnNumberToName(colIndex + 1)
			
			f.SetCellValue(sheetName, colName+"1", toko.NamaToko)
			f.MergeCell(sheetName, colName+"1", nextColName+"1")
			f.SetCellStyle(sheetName, colName+"1", nextColName+"1", headerStyle)

			f.SetCellValue(sheetName, colName+"2", "Kirim")
			f.SetCellValue(sheetName, nextColName+"2", "Retur")
			f.SetCellStyle(sheetName, colName+"2", nextColName+"2", headerStyle)
			
			f.SetColWidth(sheetName, colName, nextColName, 10)
			colIndex += 2
		}

		rowIndex := 3
		for i, barang := range uniqueBarangs {
			f.SetCellValue(sheetName, fmt.Sprintf("A%d", rowIndex), i+1)
			f.SetCellStyle(sheetName, fmt.Sprintf("A%d", rowIndex), fmt.Sprintf("A%d", rowIndex), cellBorder)
			
			f.SetCellValue(sheetName, fmt.Sprintf("B%d", rowIndex), barang)
			f.SetCellStyle(sheetName, fmt.Sprintf("B%d", rowIndex), fmt.Sprintf("B%d", rowIndex), leftBorder)

			colIdx := 3
			for _, toko := range filteredTokos {
				key := toko.NamaToko + "-" + barang
				data := cellData[key]
				
				colName, _ := excelize.ColumnNumberToName(colIdx)
				nextColName, _ := excelize.ColumnNumberToName(colIdx + 1)

				var kirimVal interface{} = ""
				if data.QtyKirim > 0 { kirimVal = data.QtyKirim }
				var returVal interface{} = ""
				if data.QtyRetur > 0 { returVal = data.QtyRetur }

				f.SetCellValue(sheetName, fmt.Sprintf("%s%d", colName, rowIndex), kirimVal)
				f.SetCellValue(sheetName, fmt.Sprintf("%s%d", nextColName, rowIndex), returVal)
				f.SetCellStyle(sheetName, fmt.Sprintf("%s%d", colName, rowIndex), fmt.Sprintf("%s%d", nextColName, rowIndex), cellBorder)
				colIdx += 2
			}
			rowIndex++
		}

		totalRow := rowIndex
		f.SetCellValue(sheetName, fmt.Sprintf("A%d", totalRow), "TOTAL")
		f.MergeCell(sheetName, fmt.Sprintf("A%d", totalRow), fmt.Sprintf("B%d", totalRow))
		f.SetCellStyle(sheetName, fmt.Sprintf("A%d", totalRow), fmt.Sprintf("B%d", totalRow), boldBorder)

		colIdx := 3
		for _, toko := range filteredTokos {
			totals := totalsMap[toko.NamaToko]
			colName, _ := excelize.ColumnNumberToName(colIdx)
			nextColName, _ := excelize.ColumnNumberToName(colIdx + 1)

			f.SetCellValue(sheetName, fmt.Sprintf("%s%d", colName, totalRow), totals.TotalKirim)
			f.SetCellStyle(sheetName, fmt.Sprintf("%s%d", colName, totalRow), fmt.Sprintf("%s%d", colName, totalRow), currencyStyle)

			f.SetCellValue(sheetName, fmt.Sprintf("%s%d", nextColName, totalRow), totals.TotalRetur)
			f.SetCellStyle(sheetName, fmt.Sprintf("%s%d", nextColName, totalRow), fmt.Sprintf("%s%d", nextColName, totalRow), currencyStyle)

			colIdx += 2
		}
	}

	if !hasData {
		f.SetSheetName("Sheet1", "Kosong")
		f.SetCellValue("Kosong", "A1", "Tidak ada data pada rentang tanggal tersebut")
	}

	c.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	
	// yyyy_mm (e.g. 2026_08)
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.xlsx", startDate.Format("2006_01")))

	if err := f.Write(c.Response().BodyWriter()); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal menulis file Excel"})
	}

	return nil
}
