package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/xuri/excelize/v2"
)

// CatatanBesarResult is used for scanning DB results
type CatatanBesarResult struct {
	NamaBarang string  `json:"nama_barang"`
	NamaToko   string  `json:"nama_toko"`
	Siklus     string  `json:"siklus"`
	IsHarian   bool    `json:"is_harian"`
	TglAsli    string  `json:"tgl_asli"`
	NotaId     int     `json:"nota_id"`
	QtyKirim   int     `json:"qty_kirim"`
	QtyRetur   int     `json:"qty_retur"`
	HargaKirim float64 `json:"harga_kirim"`
	HargaRetur float64 `json:"harga_retur"`
}

type TokoMeta struct {
	NamaToko string
	Siklus   string
}

func formatIndonesianDate(dateStr string) string {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return dateStr
	}
	
	days := []string{"Minggu", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu"}
	dayName := days[t.Weekday()]
	
	return fmt.Sprintf("%s, %s", dayName, t.Format("02-01-2006"))
}

func getSiklusPriority(siklus string) int {
	if siklus == "HARIAN" {
		return 1
	}
	if siklus == "SiklusDua" {
		return 2
	}
	return 3
}

func ExportCatatanBesar(c *fiber.Ctx) error {
	siklus := c.Query("siklus")
	tanggal := c.Query("tanggal")

	if tanggal == "" {
		tanggal = wib().Format("2006-01-02")
	}

	siklusFilter := "nota.siklus_snapshot = 'HARIAN'"
	if siklus != "" {
		if siklus == "SiklusJumatSelasa" {
			siklusFilter = "(nota.siklus_snapshot = 'SiklusJumatSelasa' OR nota.siklus_snapshot = 'SiklusDua' OR nota.siklus_snapshot = 'HARIAN')"
		} else {
			siklusFilter = fmt.Sprintf("(nota.siklus_snapshot = '%s' OR nota.siklus_snapshot = 'HARIAN')", siklus)
		}
	}

	var results []CatatanBesarResult

	kirimDateExpr := `CAST(
		CASE 
			WHEN nota.siklus_snapshot = 'HARIAN' THEN nota.tanggal_kirim
			WHEN nota.siklus_snapshot = 'SiklusDua' THEN 
				CASE 
					WHEN EXTRACT(DOW FROM nota.tanggal_kirim) IN (1,2,3) THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '1 days'
					WHEN EXTRACT(DOW FROM nota.tanggal_kirim) IN (4,5,6) THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '4 days'
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
			barangs.nama_barang, 
			tokos.nama_toko,
			MAX(nota.siklus_snapshot) as siklus,
			bool_or(nota.is_harian_snapshot) as is_harian, 
			CAST(nota.tanggal_kirim AS DATE) as tgl_asli,
			nota.id as nota_id,
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
		GROUP BY barangs.nama_barang, tokos.nama_toko, CAST(nota.tanggal_kirim AS DATE), nota.id
	`, kirimDateExpr, returDateExpr, kirimDateExpr, returDateExpr, siklusFilter)

	err := DB.Raw(query, tanggal, tanggal, tanggal, tanggal, tanggal).Scan(&results).Error
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 1. Process data: extract unique stores and unique items
	tokosMap := make(map[string]TokoMeta)
	itemsMap := make(map[string]bool)

	tokoKirimDates := make(map[string][]string)
	tokoKirimSet := make(map[string]map[string]bool)

	cellKirimData := make(map[string]int)
	cellReturData := make(map[string]int)

	kirimTotals := make(map[string]float64)
	returTotals := make(map[string]float64)

	for _, row := range results {
		if _, ok := tokosMap[row.NamaToko]; !ok {
			tokosMap[row.NamaToko] = TokoMeta{NamaToko: row.NamaToko, Siklus: row.Siklus}
		}

		if row.QtyKirim > 0 || row.QtyRetur > 0 {
			itemsMap[row.NamaBarang] = true
		}

		var sortableDateStr string
		if row.TglAsli != "" && len(row.TglAsli) >= 10 {
			t, err := time.Parse("2006-01-02T15:04:05Z", row.TglAsli)
			if err == nil {
				sortableDateStr = fmt.Sprintf("%s|%010d|%s", t.Format("2006-01-02"), row.NotaId, t.Format("02-01"))
			} else {
				t, err = time.Parse("2006-01-02", row.TglAsli[:10])
				if err == nil {
					sortableDateStr = fmt.Sprintf("%s|%010d|%s", t.Format("2006-01-02"), row.NotaId, t.Format("02-01"))
				} else {
					sortableDateStr = fmt.Sprintf("%s|%010d|%s", row.TglAsli[:10], row.NotaId, row.TglAsli[8:10]+"-"+row.TglAsli[5:7])
				}
			}
		}

		if row.QtyKirim > 0 && sortableDateStr != "" {
			if tokoKirimSet[row.NamaToko] == nil {
				tokoKirimSet[row.NamaToko] = make(map[string]bool)
			}
			if !tokoKirimSet[row.NamaToko][sortableDateStr] {
				tokoKirimSet[row.NamaToko][sortableDateStr] = true
				tokoKirimDates[row.NamaToko] = append(tokoKirimDates[row.NamaToko], sortableDateStr)
			}
		}

		if row.QtyKirim > 0 {
			kKey := row.NamaToko + "-" + row.NamaBarang + "-" + sortableDateStr
			cellKirimData[kKey] += row.QtyKirim
		}
		
		if row.QtyRetur > 0 {
			rKey := row.NamaToko + "-" + row.NamaBarang
			cellReturData[rKey] += row.QtyRetur
		}

		if sortableDateStr != "" {
			kKey := row.NamaToko + "-" + sortableDateStr
			kirimTotals[kKey] += row.HargaKirim
		}
		
		returTotals[row.NamaToko] += row.HargaRetur
	}

	for toko, dates := range tokoKirimDates {
		sort.Strings(dates)
		tokoKirimDates[toko] = dates
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

	var uniqueBarangs []string
	for item := range itemsMap {
		uniqueBarangs = append(uniqueBarangs, item)
	}
	sort.Strings(uniqueBarangs)

	// 2. Generate Excel
	f := excelize.NewFile()
	sheet := "Sheet1"
	f.SetSheetName(sheet, "Catatan Besar")
	sheet = "Catatan Besar"

	// Title
	titleText := fmt.Sprintf("Catatan Besar Tiara %s", formatIndonesianDate(tanggal))
	f.SetCellValue(sheet, "A1", titleText)
	titleStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 16},
	})
	f.SetCellStyle(sheet, "A1", "A1", titleStyle)

	headerStyle, _ := f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Color: []string{"#e2e8f0"}, Pattern: 1},
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

	// Headers
	f.SetCellValue(sheet, "A2", "Nama Produk Barang")
	f.MergeCell(sheet, "A2", "A3")
	f.SetCellStyle(sheet, "A2", "A3", headerStyle)
	f.SetColWidth(sheet, "A", "A", 30)

	colIndex := 2 // B
	for _, toko := range filteredTokos {
		kirimDates := tokoKirimDates[toko.NamaToko]
		if len(kirimDates) == 0 {
			kirimDates = []string{"Kirim"}
		}
		numCols := len(kirimDates) + 1 // +1 for Retur

		startColName, _ := excelize.ColumnNumberToName(colIndex)
		endColName, _ := excelize.ColumnNumberToName(colIndex + numCols - 1)
		
		f.SetCellValue(sheet, startColName+"2", toko.NamaToko)
		if numCols > 1 {
			f.MergeCell(sheet, startColName+"2", endColName+"2")
		}
		f.SetCellStyle(sheet, startColName+"2", endColName+"2", headerStyle)

		currCol := colIndex
		for _, compositeDate := range kirimDates {
			colName, _ := excelize.ColumnNumberToName(currCol)
			
			parts := strings.Split(compositeDate, "|")
			displayDate := ""
			if len(parts) == 3 {
				displayDate = parts[2]
			}
			
			headerText := "Kirim"
			if displayDate != "Kirim" && displayDate != "" {
				headerText = fmt.Sprintf("Kirim %s", displayDate)
			}
			f.SetCellValue(sheet, colName+"3", headerText)
			f.SetCellStyle(sheet, colName+"3", colName+"3", headerStyle)
			f.SetColWidth(sheet, colName, colName, 15)
			currCol++
		}

		returColName, _ := excelize.ColumnNumberToName(currCol)
		f.SetCellValue(sheet, returColName+"3", "Retur")
		f.SetCellStyle(sheet, returColName+"3", returColName+"3", headerStyle)
		f.SetColWidth(sheet, returColName, returColName, 15)

		colIndex += numCols
	}

	// Data
	rowIndex := 4
	for _, barang := range uniqueBarangs {
		f.SetCellValue(sheet, fmt.Sprintf("A%d", rowIndex), barang)
		f.SetCellStyle(sheet, fmt.Sprintf("A%d", rowIndex), fmt.Sprintf("A%d", rowIndex), cellBorder)

		colIdx := 2
		for _, toko := range filteredTokos {
			kirimDates := tokoKirimDates[toko.NamaToko]
			if len(kirimDates) == 0 {
				kirimDates = []string{"Kirim"}
			}

			for _, dateStr := range kirimDates {
				key := toko.NamaToko + "-" + barang + "-" + dateStr
				qtyKirim := cellKirimData[key]
				
				colName, _ := excelize.ColumnNumberToName(colIdx)
				var val interface{} = "-"
				if qtyKirim > 0 { val = qtyKirim }
				f.SetCellValue(sheet, fmt.Sprintf("%s%d", colName, rowIndex), val)
				f.SetCellStyle(sheet, fmt.Sprintf("%s%d", colName, rowIndex), fmt.Sprintf("%s%d", colName, rowIndex), cellBorder)
				colIdx++
			}

			keyRetur := toko.NamaToko + "-" + barang
			qtyRetur := cellReturData[keyRetur]
			returColName, _ := excelize.ColumnNumberToName(colIdx)
			var val interface{} = "-"
			if qtyRetur > 0 { val = qtyRetur }
			f.SetCellValue(sheet, fmt.Sprintf("%s%d", returColName, rowIndex), val)
			f.SetCellStyle(sheet, fmt.Sprintf("%s%d", returColName, rowIndex), fmt.Sprintf("%s%d", returColName, rowIndex), cellBorder)
			colIdx++
		}
		rowIndex++
	}

	// Footer
	totalRow1 := rowIndex
	totalRow2 := rowIndex + 1
	totalRow3 := rowIndex + 2

	f.SetCellValue(sheet, fmt.Sprintf("A%d", totalRow1), "Subtotal Kirim (Rp)")
	f.SetCellValue(sheet, fmt.Sprintf("A%d", totalRow2), "Total Kirim (Rp)")
	f.SetCellValue(sheet, fmt.Sprintf("A%d", totalRow3), "Total Retur (Rp)")
	f.SetCellStyle(sheet, fmt.Sprintf("A%d", totalRow1), fmt.Sprintf("A%d", totalRow3), headerStyle)

	colIdx := 2
	for _, toko := range filteredTokos {
		kirimDates := tokoKirimDates[toko.NamaToko]
		if len(kirimDates) == 0 {
			kirimDates = []string{"Kirim"}
		}
		numCols := len(kirimDates) + 1

		startColName, _ := excelize.ColumnNumberToName(colIdx)
		endColName, _ := excelize.ColumnNumberToName(colIdx + numCols - 1)

		var combinedKirim float64
		currCol := colIdx
		for _, compositeDate := range kirimDates {
			colName, _ := excelize.ColumnNumberToName(currCol)
			
			kTotal := kirimTotals[toko.NamaToko+"-"+compositeDate]
			combinedKirim += kTotal
			f.SetCellValue(sheet, fmt.Sprintf("%s%d", colName, totalRow1), kTotal)
			f.SetCellStyle(sheet, fmt.Sprintf("%s%d", colName, totalRow1), fmt.Sprintf("%s%d", colName, totalRow1), currencyStyle)
			
			currCol++
		}

		returColName, _ := excelize.ColumnNumberToName(currCol)
		
		// Row 1: Blank for Retur column under Subtotal Kirim
		f.SetCellValue(sheet, fmt.Sprintf("%s%d", returColName, totalRow1), "")
		f.SetCellStyle(sheet, fmt.Sprintf("%s%d", returColName, totalRow1), fmt.Sprintf("%s%d", returColName, totalRow1), currencyStyle)

		// Row 2: Total Kirim Keseluruhan (di-merge)
		if numCols > 1 {
			f.MergeCell(sheet, fmt.Sprintf("%s%d", startColName, totalRow2), fmt.Sprintf("%s%d", endColName, totalRow2))
		}
		f.SetCellValue(sheet, fmt.Sprintf("%s%d", startColName, totalRow2), combinedKirim)
		f.SetCellStyle(sheet, fmt.Sprintf("%s%d", startColName, totalRow2), fmt.Sprintf("%s%d", endColName, totalRow2), currencyStyle)

		// Row 3: Total Retur (di-merge)
		if numCols > 1 {
			f.MergeCell(sheet, fmt.Sprintf("%s%d", startColName, totalRow3), fmt.Sprintf("%s%d", endColName, totalRow3))
		}
		rTotal := returTotals[toko.NamaToko]
		f.SetCellValue(sheet, fmt.Sprintf("%s%d", startColName, totalRow3), rTotal)
		f.SetCellStyle(sheet, fmt.Sprintf("%s%d", startColName, totalRow3), fmt.Sprintf("%s%d", endColName, totalRow3), currencyStyle)

		colIdx += numCols
	}

	c.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=Catatan_Besar_%s.xlsx", tanggal))

	if err := f.Write(c.Response().BodyWriter()); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal menulis file Excel"})
	}

	return nil
}

type CatatanPesananResult struct {
	NamaBarangBebas  string  `json:"nama_barang_bebas"`
	NamaTokoSnapshot string  `json:"nama_toko"`
	JenisPengambilan string  `json:"jenis_pengambilan"`
	TotalBanyak      int     `json:"total_banyak"`
	TotalRupiah      float64 `json:"total_rupiah"`
}

func ExportCatatanPesanan(c *fiber.Ctx) error {
	tgl := c.Query("tanggal")

	if tgl == "" {
		tgl = wib().Format("2006-01-02")
	}

	var results []CatatanPesananResult

	err := DB.Table("nota_pesanan_details").
		Select("nota_pesanan_details.nama_barang_bebas, nota_pesanans.nama_toko_snapshot, nota_pesanans.jenis_pengambilan, SUM(nota_pesanan_details.banyak) as total_banyak, SUM(nota_pesanan_details.subtotal) as total_rupiah").
		Joins("join nota_pesanans on nota_pesanans.id = nota_pesanan_details.nota_pesanan_id").
		Where("DATE(nota_pesanans.tanggal_kirim) = ?", tgl).
		Group("nota_pesanan_details.nama_barang_bebas, nota_pesanans.nama_toko_snapshot, nota_pesanans.jenis_pengambilan").
		Scan(&results).Error

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	tokosMap := make(map[string]bool)
	barangsMap := make(map[string]map[string]int) // item -> toko -> qty
	totalRupiahMap := make(map[string]float64) // toko -> total rupiah

	for _, row := range results {
		tokosMap[row.NamaTokoSnapshot] = true

		if barangsMap[row.NamaBarangBebas] == nil {
			barangsMap[row.NamaBarangBebas] = make(map[string]int)
		}
		barangsMap[row.NamaBarangBebas][row.NamaTokoSnapshot] += row.TotalBanyak
		totalRupiahMap[row.NamaTokoSnapshot] += row.TotalRupiah
	}

	var filteredTokos []string
	for t := range tokosMap {
		filteredTokos = append(filteredTokos, t)
	}
	sort.Strings(filteredTokos)

	var uniqueBarangs []string
	for b := range barangsMap {
		uniqueBarangs = append(uniqueBarangs, b)
	}
	sort.Strings(uniqueBarangs)

	f := excelize.NewFile()
	sheet := "Sheet1"
	f.SetSheetName(sheet, "Catatan Pesanan")
	sheet = "Catatan Pesanan"

	// Title
	titleText := fmt.Sprintf("Catatan Pesanan Tiara %s", formatIndonesianDate(tgl))
	f.SetCellValue(sheet, "A1", titleText)
	titleStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 16},
	})
	f.SetCellStyle(sheet, "A1", "A1", titleStyle)

	headerStyle, _ := f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Color: []string{"#eab308"}, Pattern: 1}, // Yellow-500
		Font: &excelize.Font{Bold: true, Color: "#ffffff"},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border: []excelize.Border{
			{Type: "left", Color: "#000000", Style: 1},
			{Type: "top", Color: "#000000", Style: 1},
			{Type: "bottom", Color: "#000000", Style: 1},
			{Type: "right", Color: "#000000", Style: 1},
		},
	})

	cellBorder, _ := f.NewStyle(&excelize.Style{
		Border: []excelize.Border{
			{Type: "left", Color: "#000000", Style: 1},
			{Type: "top", Color: "#000000", Style: 1},
			{Type: "bottom", Color: "#000000", Style: 1},
			{Type: "right", Color: "#000000", Style: 1},
		},
	})

	totalColStyle, _ := f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Color: []string{"#f1f5f9"}, Pattern: 1}, // Slate-100
		Font: &excelize.Font{Bold: true, Color: "#1e40af"},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
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
		Border: []excelize.Border{
			{Type: "left", Color: "#000000", Style: 1},
			{Type: "top", Color: "#000000", Style: 1},
			{Type: "bottom", Color: "#000000", Style: 1},
			{Type: "right", Color: "#000000", Style: 1},
		},
	})

	// Headers
	f.SetCellValue(sheet, "A4", "Roti Pesanan")
	f.SetCellStyle(sheet, "A4", "A4", headerStyle)
	f.SetColWidth(sheet, "A", "A", 30)

	colIndex := 2 // B
	for _, toko := range filteredTokos {
		colName, _ := excelize.ColumnNumberToName(colIndex)
		f.SetCellValue(sheet, colName+"4", toko)
		f.SetCellStyle(sheet, colName+"4", colName+"4", headerStyle)
		f.SetColWidth(sheet, colName, colName, 15)
		colIndex++
	}

	totalColName, _ := excelize.ColumnNumberToName(colIndex)
	f.SetCellValue(sheet, totalColName+"4", "Total Produksi")
	f.SetCellStyle(sheet, totalColName+"4", totalColName+"4", headerStyle)
	f.SetColWidth(sheet, totalColName, totalColName, 20)

	// Data
	rowIndex := 5
	for _, barang := range uniqueBarangs {
		f.SetCellValue(sheet, fmt.Sprintf("A%d", rowIndex), barang)
		f.SetCellStyle(sheet, fmt.Sprintf("A%d", rowIndex), fmt.Sprintf("A%d", rowIndex), cellBorder)

		colIdx := 2
		rowTotal := 0
		for _, toko := range filteredTokos {
			qty := barangsMap[barang][toko]
			rowTotal += qty
			
			colName, _ := excelize.ColumnNumberToName(colIdx)
			var val interface{} = "-"
			if qty > 0 {
				val = qty
			}
			f.SetCellValue(sheet, fmt.Sprintf("%s%d", colName, rowIndex), val)
			f.SetCellStyle(sheet, fmt.Sprintf("%s%d", colName, rowIndex), fmt.Sprintf("%s%d", colName, rowIndex), cellBorder)
			colIdx++
		}

		tColName, _ := excelize.ColumnNumberToName(colIdx)
		f.SetCellValue(sheet, fmt.Sprintf("%s%d", tColName, rowIndex), rowTotal)
		f.SetCellStyle(sheet, fmt.Sprintf("%s%d", tColName, rowIndex), fmt.Sprintf("%s%d", tColName, rowIndex), totalColStyle)

		rowIndex++
	}

	// Footer (Omzet)
	f.SetCellValue(sheet, fmt.Sprintf("A%d", rowIndex), "Total Omzet (Rp)")
	f.SetCellStyle(sheet, fmt.Sprintf("A%d", rowIndex), fmt.Sprintf("A%d", rowIndex), headerStyle)

	colIdx := 2
	grandTotalRupiah := 0.0
	for _, toko := range filteredTokos {
		omzet := totalRupiahMap[toko]
		grandTotalRupiah += omzet
		
		colName, _ := excelize.ColumnNumberToName(colIdx)
		f.SetCellValue(sheet, fmt.Sprintf("%s%d", colName, rowIndex), omzet)
		f.SetCellStyle(sheet, fmt.Sprintf("%s%d", colName, rowIndex), fmt.Sprintf("%s%d", colName, rowIndex), currencyStyle)
		colIdx++
	}

	tColName, _ := excelize.ColumnNumberToName(colIdx)
	f.SetCellValue(sheet, fmt.Sprintf("%s%d", tColName, rowIndex), grandTotalRupiah)
	f.SetCellStyle(sheet, fmt.Sprintf("%s%d", tColName, rowIndex), fmt.Sprintf("%s%d", tColName, rowIndex), currencyStyle)

	c.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=Catatan_Pesanan_%s.xlsx", tgl))

	if err := f.Write(c.Response().BodyWriter()); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal menulis file Excel"})
	}

	return nil
}
