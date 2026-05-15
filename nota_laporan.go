package main

import (
	"backend/models"
	"fmt"
	"sort"
	"time"

	"github.com/gofiber/fiber/v2"
)

// CATATAN BESAR
func GetCatatanBesar(c *fiber.Ctx) error {
	siklus := c.Query("siklus")
	tanggal := c.Query("tanggal")

	if tanggal == "" {
		tanggal = time.Now().Format("2006-01-02")
	}

	// Filter dinamis: Jika siklus kosong (misal hari Minggu), HANYA cari toko harian
	siklusFilter := "nota.siklus_snapshot = 'HARIAN'"
	if siklus != "" {
		if siklus == "SiklusJumatSelasa" {
			// Jika memilih Selasa/Jumat, tarik data JumatSelasa DAN SiklusDua
			siklusFilter = "(nota.siklus_snapshot = 'SiklusJumatSelasa' OR nota.siklus_snapshot = 'SiklusDua' OR nota.siklus_snapshot = 'HARIAN')"
		} else {
			siklusFilter = fmt.Sprintf("(nota.siklus_snapshot = '%s' OR nota.siklus_snapshot = 'HARIAN')", siklus)
		}
	}

	var results []struct {
		NamaBarang string  `json:"nama_barang"`
		NamaToko   string  `json:"nama_toko"`
		Siklus     string  `json:"siklus"`
		IsHarian   bool    `json:"is_harian"` // Agar Vue tahu opacity-nya
		QtyKirim   int     `json:"qty_kirim"`
		QtyRetur   int     `json:"qty_retur"`
		HargaKirim float64 `json:"harga_kirim"`
		HargaRetur float64 `json:"harga_retur"`
	}

	// INI YANG TADI NGGAK SENGAJA TERHAPUS! KITA KEMBALIKAN!
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

	// Query cerdas: Tarik kerangka 30 hari ke belakang agar tabel stabil,
	// tapi nilai SUM dikunci ketat HANYA pada tanggal filter.
	query := fmt.Sprintf(`
		SELECT 
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
		GROUP BY barangs.nama_barang, tokos.nama_toko
	`, kirimDateExpr, returDateExpr, kirimDateExpr, returDateExpr, siklusFilter)

	// Melempar 5 parameter tanggal
	err := DB.Raw(query, tanggal, tanggal, tanggal, tanggal, tanggal).Scan(&results).Error

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(results)
}

// RANGKUMAN (Logika Anchor Day / Hari Jangkar Mutlak)
func GetRangkuman(c *fiber.Ctx) error {
	start := c.Query("start")
	end := c.Query("end")

	if start == "" || end == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Tanggal start dan end wajib diisi"})
	}

	startDate, _ := time.Parse("2006-01-02", start)
	endDate, _ := time.Parse("2006-01-02", end)

	// 1. AMBIL SEMUA TOKO
	var semuaToko []models.Toko
	DB.Unscoped().Find(&semuaToko)

	rekapMap := make(map[uint]*models.RekapToko)
	for _, t := range semuaToko {
		rekapMap[t.ID] = &models.RekapToko{ID: t.ID, Nama: t.NamaToko, Kirim: 0, Retur: 0, Diskon: 0, Pendapatan: 0, Persentase: 0}
	}

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
				CASE EXTRACT(DOW FROM nota.tanggal_kirim)
					WHEN 1 THEN DATE_TRUNC('week', nota.tanggal_kirim) - INTERVAL '4 days'
					WHEN 2 THEN DATE_TRUNC('week', nota.tanggal_kirim) - INTERVAL '3 days'
					WHEN 3 THEN DATE_TRUNC('week', nota.tanggal_kirim) - INTERVAL '2 days'
					WHEN 4 THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '0 days'
					WHEN 5 THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '1 days'
					WHEN 6 THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '2 days'
					WHEN 0 THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '2 days' 
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

	var rawResults []struct {
		ID     uint
		Nama   string
		Kirim  float64
		Retur  float64
		Diskon float64
	}

	// 3. EKSEKUSI SQL TOKO (Menambahkan penarikan Diskon + Voucher)
	queryToko := fmt.Sprintf(`
		SELECT 
			toko_id as id,
			MAX(nama_toko_snapshot) as nama,
			COALESCE(SUM(CASE WHEN %[1]s >= CAST(? AS DATE) AND %[1]s <= CAST(? AS DATE) THEN jumlah_kirim ELSE 0 END), 0) as kirim,
			COALESCE(SUM(CASE WHEN %[2]s >= CAST(? AS DATE) AND %[2]s <= CAST(? AS DATE) THEN jumlah_retur ELSE 0 END), 0) as retur,
			COALESCE(SUM(CASE WHEN %[1]s >= CAST(? AS DATE) AND %[1]s <= CAST(? AS DATE) THEN (total_diskon + total_voucher) ELSE 0 END), 0) as diskon
		FROM nota
		WHERE
			( 
				(%[1]s >= CAST(? AS DATE) AND %[1]s <= CAST(? AS DATE))
				OR 
				(%[2]s >= CAST(? AS DATE) AND %[2]s <= CAST(? AS DATE))
			)
			AND nota.status != 'DIBATALKAN'
		GROUP BY toko_id
	`, kirimDateExpr, returDateExpr)

	DB.Raw(queryToko, startDate, endDate, startDate, endDate, startDate, endDate, startDate, endDate, startDate, endDate).Scan(&rawResults)

	var totalKirim, totalRetur, totalDiskon float64

	for _, r := range rawResults {
		if val, exists := rekapMap[r.ID]; exists {
			val.Kirim = r.Kirim
			val.Retur = r.Retur
			val.Diskon = r.Diskon
			val.Pendapatan = r.Kirim - r.Retur - r.Diskon // <--- Potong dengan diskon riil

			// LOGIKA BISNIS: Jika ada kirim, hitung normal. Jika tidak ada kirim tapi ada retur, SET 100%!
			if r.Kirim > 0 {
				val.Persentase = (r.Retur / r.Kirim) * 100
			} else if r.Retur > 0 {
				val.Persentase = 100
			} else {
				val.Persentase = 0
			}

			if r.Nama != "" {
				val.Nama = r.Nama
			}
		}
		totalKirim += r.Kirim
		totalRetur += r.Retur
		totalDiskon += r.Diskon
	}

	var perToko []models.RekapToko
	for _, r := range rekapMap {
		perToko = append(perToko, *r)
	}

	sort.Slice(perToko, func(i, j int) bool { return perToko[i].Pendapatan > perToko[j].Pendapatan })

	totalPersentase := 0.0
	if totalKirim > 0 {
		totalPersentase = (totalRetur / totalKirim) * 100
	} else if totalRetur > 0 {
		totalPersentase = 100
	}

	var rawBarang []struct {
		Nama  string
		Kirim float64
		Retur float64
	}

	// 6. EKSEKUSI SQL BARANG
	queryBarang := fmt.Sprintf(`
		SELECT 
			MAX(nota_details.nama_barang_snapshot) as nama,
			COALESCE(SUM(CASE WHEN %s >= CAST(? AS DATE) AND %s <= CAST(? AS DATE) THEN nota_details.banyak_kirim ELSE 0 END), 0) as kirim,
			COALESCE(SUM(CASE WHEN %s >= CAST(? AS DATE) AND %s <= CAST(? AS DATE) THEN nota_details.banyak_retur ELSE 0 END), 0) as retur
		FROM nota_details
		JOIN nota ON nota.id = nota_details.nota_id
		WHERE
			(
				(%s >= CAST(? AS DATE) AND %s <= CAST(? AS DATE))
				OR 
				(%s >= CAST(? AS DATE) AND %s <= CAST(? AS DATE))
			)
			AND nota.status != 'DIBATALKAN'
		GROUP BY nota_details.barang_id
	`, kirimDateExpr, kirimDateExpr, returDateExpr, returDateExpr, kirimDateExpr, kirimDateExpr, returDateExpr, returDateExpr)

	DB.Raw(queryBarang, startDate, endDate, startDate, endDate, startDate, endDate, startDate, endDate).Scan(&rawBarang)

	var perBarang []models.RekapBarang
	for _, b := range rawBarang {
		if b.Kirim == 0 && b.Retur == 0 {
			continue
		}

		persen := 0.0
		if b.Kirim > 0 {
			persen = (b.Retur / b.Kirim) * 100
		} else if b.Retur > 0 {
			persen = 100
		}

		perBarang = append(perBarang, models.RekapBarang{
			Nama:       b.Nama,
			QtyKirim:   b.Kirim,
			QtyRetur:   b.Retur,
			QtyLaku:    b.Kirim - b.Retur,
			Persentase: persen,
		})
	}

	sort.Slice(perBarang, func(i, j int) bool { return perBarang[i].QtyLaku > perBarang[j].QtyLaku })

	return c.JSON(models.RangkumanResponse{
		Kirim:      totalKirim,
		Retur:      totalRetur,
		Diskon:     totalDiskon,
		Pendapatan: totalKirim - totalRetur - totalDiskon, // <--- Hasil Kas Riil
		Persentase: totalPersentase,
		PerToko:    perToko,
		PerBarang:  perBarang,
	})
}

func GetRangkumanPerToko(c *fiber.Ctx) error {
	start := c.Query("start")
	end := c.Query("end")
	tokoID := c.Query("toko_id")

	if tokoID == "" || tokoID == "null" || tokoID == "undefined" {
		return c.Status(400).JSON(fiber.Map{"error": "ID Toko tidak boleh kosong"})
	}

	var hasil []struct {
		NamaBarang string  `json:"nama_barang"`
		TotalKirim int     `json:"total_kirim"`
		TotalRetur int     `json:"total_retur"`
		TotalLaku  int     `json:"total_laku"`
		Persentase float64 `json:"persentase"`
	}

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
				CASE EXTRACT(DOW FROM nota.tanggal_kirim)
					WHEN 1 THEN DATE_TRUNC('week', nota.tanggal_kirim) - INTERVAL '4 days'
					WHEN 2 THEN DATE_TRUNC('week', nota.tanggal_kirim) - INTERVAL '3 days'
					WHEN 3 THEN DATE_TRUNC('week', nota.tanggal_kirim) - INTERVAL '2 days'
					WHEN 4 THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '0 days'
					WHEN 5 THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '1 days'
					WHEN 6 THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '2 days'
					WHEN 0 THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '2 days'
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
			MAX(nota_details.nama_barang_snapshot) as nama_barang, 
			COALESCE(SUM(CASE WHEN %s >= CAST(? AS DATE) AND %s <= CAST(? AS DATE) THEN nota_details.banyak_kirim ELSE 0 END), 0) as total_kirim, 
			COALESCE(SUM(CASE WHEN %s >= CAST(? AS DATE) AND %s <= CAST(? AS DATE) THEN nota_details.banyak_retur ELSE 0 END), 0) as total_retur
		FROM nota_details
		JOIN nota ON nota.id = nota_details.nota_id
		WHERE 
			((%s >= CAST(? AS DATE) AND %s <= CAST(? AS DATE))
			OR 
			(%s >= CAST(? AS DATE) AND %s <= CAST(? AS DATE)))
		AND nota.toko_id = ?
		AND nota.status != 'DIBATALKAN'
		GROUP BY nota_details.barang_id
	`, kirimDateExpr, kirimDateExpr, returDateExpr, returDateExpr, kirimDateExpr, kirimDateExpr, returDateExpr, returDateExpr)

	DB.Raw(query, start, end, start, end, start, end, start, end, tokoID).Scan(&hasil)

	for i := range hasil {
		hasil[i].TotalLaku = hasil[i].TotalKirim - hasil[i].TotalRetur

		// LOGIKA BISNIS: Kirim 0 tapi Retur > 0 = 100%
		if hasil[i].TotalKirim > 0 {
			hasil[i].Persentase = (float64(hasil[i].TotalRetur) / float64(hasil[i].TotalKirim)) * 100
		} else if hasil[i].TotalRetur > 0 {
			hasil[i].Persentase = 100
		} else {
			hasil[i].Persentase = 0
		}
	}

	sort.Slice(hasil, func(i, j int) bool { return hasil[i].TotalLaku > hasil[j].TotalLaku })

	return c.JSON(hasil)
}

// 3. Get Rangkuman Khusus Pesanan (Untuk Tab Rangkuman Bulanan)
func GetRangkumanPesanan(c *fiber.Ctx) error {
	start := c.Query("start")
	end := c.Query("end")

	// Total Global
	var summary struct {
		TotalPendapatan float64 `json:"total_pendapatan"`
		TotalPesanan    int     `json:"total_pesanan"`
		TotalDiskon     float64 `json:"total_diskon"`
	}
	// LOGIKA SIMPLE: Semua omzet (dikurangi voucher) diakui penuh di hari H pengiriman
	DB.Model(&models.NotaPesanan{}).
		Where("tanggal_kirim >= ? AND tanggal_kirim <= ? AND status != 'DIBATALKAN'", start, end).
		Select("COALESCE(SUM(total_bayar - total_voucher), 0) as total_pendapatan, COALESCE(SUM(total_voucher), 0) as total_diskon, COUNT(id) as total_pesanan").
		Scan(&summary)

	// Breakdown Per Titik Ambil
	var perTitik []struct {
		NamaTitik  string  `json:"nama_titik"`
		Pendapatan float64 `json:"pendapatan"`
		Diskon     float64 `json:"diskon"`
		TotalNota  int     `json:"total_nota"`
	}
	DB.Model(&models.NotaPesanan{}).
		Where("tanggal_kirim >= ? AND tanggal_kirim <= ? AND status != 'DIBATALKAN'", start, end).
		Select("nama_toko_snapshot as nama_titik, COALESCE(SUM(total_bayar - total_voucher), 0) as pendapatan, COALESCE(SUM(total_voucher), 0) as diskon, COUNT(id) as total_nota").
		Group("nama_toko_snapshot").
		Order("pendapatan desc").
		Scan(&perTitik)

	// Detail Pesanan per Barang
	var detailBarang []struct {
		NamaTitik   string  `json:"nama_titik"`
		NamaBarang  string  `json:"nama_barang"`
		TotalQty    int     `json:"total_qty"`
		TotalRupiah float64 `json:"total_rupiah"`
	}
	DB.Table("nota_pesanan_details").
		Select("nota_pesanans.nama_toko_snapshot as nama_titik, nota_pesanan_details.nama_barang_bebas as nama_barang, SUM(nota_pesanan_details.banyak) as total_qty, SUM(nota_pesanan_details.subtotal) as total_rupiah").
		Joins("join nota_pesanans on nota_pesanans.id = nota_pesanan_details.nota_pesanan_id").
		Where("nota_pesanans.tanggal_kirim >= ? AND nota_pesanans.tanggal_kirim <= ? AND nota_pesanans.status != 'DIBATALKAN'", start, end).
		Group("nota_pesanans.nama_toko_snapshot, nota_pesanan_details.nama_barang_bebas").
		Order("nama_titik asc, total_qty desc").
		Scan(&detailBarang)

	// Kembalikan datanya ke Vue
	return c.JSON(fiber.Map{
		"total_pendapatan": summary.TotalPendapatan,
		"total_pesanan":    summary.TotalPesanan,
		"total_diskon":     summary.TotalDiskon, // Murni Voucher saja
		"per_titik":        perTitik,
		"detail_barang":    detailBarang,
	})
}
