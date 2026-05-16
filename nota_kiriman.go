package main

import (
	"backend/models"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// BUAT NOTA
func GetNextNotaNumber(c *fiber.Ctx) error {
	tokoID := c.Query("toko_id")
	tgl := c.Query("tanggal") // Format: 2026-04-27
	tglStr := strings.ReplaceAll(tgl, "-", "")

	var notaTerakhir models.Nota
	// Cari 1 nota terakhir yang dibuat untuk toko ini (berdasarkan ID terbesar)
	err := DB.Unscoped().Where("toko_id = ?", tokoID).Order("id desc").First(&notaTerakhir).Error

	nextUrutan := 1
	if err == nil && notaTerakhir.NoNota != "" {
		// Asumsi format: NT/20260427/15-0017
		// Kita pisahkan berdasarkan tanda strip "-"
		parts := strings.Split(notaTerakhir.NoNota, "-")
		if len(parts) > 1 {
			// Ambil bagian paling belakang (contoh: "0017")
			lastNumStr := parts[len(parts)-1]
			if lastNum, errParse := strconv.Atoi(lastNumStr); errParse == nil {
				nextUrutan = lastNum + 1 // 17 + 1 = 18
			}
		}
	} else {
		// Fallback jika belum ada nota sama sekali untuk toko ini
		var count int64
		DB.Unscoped().Model(&models.Nota{}).Where("toko_id = ?", tokoID).Count(&count)
		nextUrutan = int(count) + 1
	}

	// Format: NT/20260427/1-0001
	// %04d berarti angka akan diformat menjadi 4 digit (0001)
	noNota := fmt.Sprintf("NT/%s/%s-%04d", tglStr, tokoID, nextUrutan)

	return c.JSON(fiber.Map{"no_nota": noNota})
}

func CreateNota(c *fiber.Ctx) error {
	var input struct {
		NoNota       string `json:"no_nota"`
		TokoID       uint   `json:"toko_id"`
		TanggalKirim string `json:"tanggal_kirim"`
		AssignedTo   uint   `json:"assigned_to"`
		Status       string `json:"status"`
		IsLunas      bool   `json:"is_lunas"`
		Details      []struct {
			BarangID    uint `json:"barang_id"`
			BanyakKirim int  `json:"banyak_kirim"`
		} `json:"details"`
	}

	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format data salah"})
	}

	var toko models.Toko
	if err := DB.First(&toko, input.TokoID).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Toko tidak ditemukan"})
	}

	tgl, _ := time.Parse("2006-01-02", input.TanggalKirim)
	hari := tgl.Weekday()

	// Logika Penentuan Siklus Snapshot
	var siklusAktif string

	if toko.IsHarian {
		siklusAktif = "HARIAN"
	} else if toko.SiklusDua {
		siklusAktif = "SiklusDua"
	} else {
		switch {
		case hari == time.Thursday && toko.SiklusKamisSenin:
			siklusAktif = "SiklusKamisSenin"
		case hari == time.Friday && toko.SiklusJumatSelasa:
			siklusAktif = "SiklusJumatSelasa"
		case hari == time.Saturday && toko.SiklusSabtuRabu:
			siklusAktif = "SiklusSabtuRabu"
		default:
			if toko.SiklusKamisSenin {
				siklusAktif = "SiklusKamisSenin"
			} else if toko.SiklusJumatSelasa {
				siklusAktif = "SiklusJumatSelasa"
			} else if toko.SiklusSabtuRabu {
				siklusAktif = "SiklusSabtuRabu"
			}
		}
	}

	adminID := c.Locals("admin_id").(uint) // Ambil ID dari token
	role := c.Locals("role").(string)      // Ambil role yang sedang login

	// LOGIKA OTOMATIS ASSIGNED
	var assignedTo uint = input.AssignedTo
	if role == "sales" {
		// Jika yang buat sales, dia otomatis jadi penanggung jawab (AssignedTo)
		assignedTo = adminID
	}

	// LOGIKA STATUS AWAL
	statusAwal := "KIRIM"
	if input.Status != "" {
		statusAwal = input.Status
	}

	nota := models.Nota{
		NoNota:           input.NoNota,
		TokoID:           input.TokoID,
		TanggalKirim:     tgl,
		Status:           statusAwal,
		NamaTokoSnapshot: toko.NamaToko,
		SiklusSnapshot:   siklusAktif,
		IsHarianSnapshot: toko.IsHarian,
		CreatedBy:        adminID,
		AssignedTo:       assignedTo,
		IsLunas:          input.IsLunas,
	}

	var totalKirim float64
	for _, d := range input.Details {
		var barang models.Barang
		if err := DB.First(&barang, d.BarangID).Error; err == nil {
			subtotal := float64(d.BanyakKirim) * barang.HargaDefault
			totalKirim += subtotal

			nota.Details = append(nota.Details, models.NotaDetail{
				BarangID:           d.BarangID,
				NamaBarangSnapshot: barang.NamaBarang,
				BanyakKirim:        d.BanyakKirim,
				HargaJual:          barang.HargaDefault,
				HargaKirim:         subtotal,
			})
		}
	}

	nota.JumlahKirim = totalKirim
	nota.TotalBayar = totalKirim

	if err := DB.Create(&nota).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// ==========================================
	// FULL SYNC KAS: CREATE NOTA LUNAS
	// ==========================================
	var settingKas models.PengaturanSistem
	DB.Where("key = ?", "ENABLE_KAS_SYNC").First(&settingKas)
	if settingKas.Value == "true" {
		if input.IsLunas {
			DB.Create(&models.TransaksiKas{
				Tanggal:    time.Now(),
				Kategori:   "REGULER",
				Jenis:      "MASUK",
				Nominal:    totalKirim,
				Keterangan: fmt.Sprintf("Pelunasan Reguler - %s (Toko: %s)", nota.NoNota, toko.NamaToko),
				NoNotaRef:  nota.NoNota,
				CreatedBy:  adminID,
			})
		}
	}
	return c.JSON(fiber.Map{"message": "Nota berhasil dibuat!", "id": nota.ID})
}

func UpdateNota(c *fiber.Ctx) error {
	id := c.Params("id")
	var input struct {
		TanggalKirim string  `json:"tanggal_kirim"`
		AssignedTo   uint    `json:"assigned_to"`
		Status       string  `json:"status"`
		IsLunas      bool    `json:"is_lunas"`
		TotalDiskon  float64 `json:"total_diskon"`
		TotalVoucher float64 `json:"total_voucher"`
		Details      []struct {
			ID          uint    `json:"id"`
			BarangID    uint    `json:"barang_id"`
			BanyakKirim int     `json:"banyak_kirim"` // <--- 1. TAMBAHKAN PENANGKAP QTY KIRIM
			BanyakRetur int     `json:"banyak_retur"`
			HargaJual   float64 `json:"harga_jual"`
		} `json:"details"`
	}

	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	tglBaru, errDate := time.Parse("2006-01-02", input.TanggalKirim)
	if errDate != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format tanggal tidak valid"})
	}

	for _, d := range input.Details {
		hRetur := float64(d.BanyakRetur) * d.HargaJual
		hKirim := float64(d.BanyakKirim) * d.HargaJual // <--- 2. HITUNG ULANG HARGA KIRIM

		if d.ID != 0 {
			// Kasus 1: Detail sudah ada, update KIRIM dan RETUR
			DB.Model(&models.NotaDetail{}).Where("id = ?", d.ID).Updates(map[string]interface{}{
				"banyak_kirim": d.BanyakKirim, // <--- 3. SIMPAN QTY KIRIM BARU
				"harga_kirim":  hKirim,        // <--- 4. SIMPAN HARGA KIRIM BARU
				"banyak_retur": d.BanyakRetur,
				"harga_retur":  hRetur,
			})
		} else if d.BanyakRetur > 0 || d.BanyakKirim > 0 {
			// Kasus 2: Tambah baris baru jika ada isian kirim/retur baru
			var barang models.Barang
			DB.First(&barang, d.BarangID)

			parsedID, err := strconv.Atoi(id)
			if err != nil {
				return c.Status(400).JSON(fiber.Map{"error": "ID Nota tidak valid"})
			}

			newDetail := models.NotaDetail{
				NotaID:             uint(parsedID),
				BarangID:           d.BarangID,
				NamaBarangSnapshot: barang.NamaBarang,
				BanyakKirim:        d.BanyakKirim, // <--- PAKAI INPUT VUE
				HargaJual:          d.HargaJual,
				HargaKirim:         hKirim, // <--- PAKAI HITUNGAN BARU
				BanyakRetur:        d.BanyakRetur,
				HargaRetur:         hRetur,
			}
			DB.Create(&newDetail)
		}
	}

	// Hitung ulang total Kirim & Retur untuk Header Nota
	var totalKirim, totalRetur float64
	DB.Model(&models.NotaDetail{}).Where("nota_id = ?", id).Select("COALESCE(SUM(harga_kirim), 0)").Row().Scan(&totalKirim)
	DB.Model(&models.NotaDetail{}).Where("nota_id = ?", id).Select("COALESCE(SUM(harga_retur), 0)").Row().Scan(&totalRetur)

	// LOGIKA UANG RIIL
	totalBayarAkhir := totalKirim - totalRetur - input.TotalDiskon - input.TotalVoucher

	DB.Model(&models.Nota{}).Where("id = ?", id).Updates(map[string]interface{}{
		"tanggal_kirim": tglBaru,
		"jumlah_kirim":  totalKirim, // <--- 5. WAJIB UPDATE TOTAL KIRIM DI HEADER
		"jumlah_retur":  totalRetur,
		"total_diskon":  input.TotalDiskon,
		"total_voucher": input.TotalVoucher,
		"total_bayar":   totalBayarAkhir,
		"assigned_to":   input.AssignedTo,
		"status":        input.Status,
		"is_lunas":      input.IsLunas,
	})

	// ==========================================
	// FULL SYNC KAS: UPDATE NOTA REGULER
	// ==========================================
	var settingKas models.PengaturanSistem
	DB.Where("key = ?", "ENABLE_KAS_SYNC").First(&settingKas)

	if settingKas.Value == "true" {
		var notaLama models.Nota
		DB.First(&notaLama, id)

		adminID := c.Locals("admin_id").(uint)
		var kasReguler models.TransaksiKas
		errKas := DB.Where("no_nota_ref = ? AND kategori = 'REGULER'", notaLama.NoNota).First(&kasReguler).Error

		if input.IsLunas {
			ket := fmt.Sprintf("Pelunasan Reguler - %s (Toko: %s)", notaLama.NoNota, notaLama.NamaTokoSnapshot)
			if errKas == nil {
				// Sudah ada kasnya, UPDATE nominalnya (Bisa jadi ada tambahan retur/diskon)
				DB.Model(&kasReguler).Updates(map[string]interface{}{
					"nominal":    totalBayarAkhir,
					"keterangan": ket,
				})
			} else {
				// Belum ada, CREATE kas masuk
				DB.Create(&models.TransaksiKas{
					Tanggal:    time.Now(),
					Kategori:   "REGULER",
					Jenis:      "MASUK",
					Nominal:    totalBayarAkhir,
					Keterangan: ket,
					NoNotaRef:  notaLama.NoNota,
					CreatedBy:  adminID,
				})
			}
		} else {
			// Jika TIDAK LUNAS (atau Batal Lunas), HAPUS KAS JIKA ADA!
			if errKas == nil {
				DB.Unscoped().Delete(&kasReguler)
			}
		}
	}

	return c.JSON(fiber.Map{"message": "Nota dan Qty Kirim berhasil diupdate!"})
}

// Batalkan Nota Reguler (Soft Delete & Tarik Kas)
func BatalkanNota(c *fiber.Ctx) error {
	id := c.Params("id")
	tx := DB.Begin()

	var nota models.Nota
	if err := tx.First(&nota, id).Error; err != nil {
		tx.Rollback()
		return c.Status(404).JSON(fiber.Map{"error": "Nota tidak ditemukan"})
	}

	// 1. Ubah status jadi DIBATALKAN
	if err := tx.Model(&nota).Update("status", "DIBATALKAN").Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 2. Tarik uang kembali dari Brankas (Otomatis hapus Kas)
	tx.Unscoped().Where("no_nota_ref = ?", nota.NoNota).Delete(&models.TransaksiKas{})

	tx.Commit()
	return c.JSON(fiber.Map{"message": "Nota berhasil dibatalkan dan Kas ditarik kembali!"})
}

// PULIHKAN NOTA REGULER
func PulihkanNota(c *fiber.Ctx) error {
	id := c.Params("id")
	tx := DB.Begin()

	var nota models.Nota
	if err := tx.First(&nota, id).Error; err != nil {
		tx.Rollback()
		return c.Status(404).JSON(fiber.Map{"error": "Nota tidak ditemukan"})
	}

	// 1. Kembalikan status menjadi aktif (KIRIM)
	if err := tx.Model(&nota).Update("status", "KIRIM").Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 2. Kembalikan Kas (Uang Masuk Lagi) Jika Nota Lunas
	var settingKas models.PengaturanSistem
	tx.Where("key = ?", "ENABLE_KAS_SYNC").First(&settingKas)

	if settingKas.Value == "true" && nota.IsLunas {
		adminID := c.Locals("admin_id").(uint)
		tx.Create(&models.TransaksiKas{
			Tanggal:    time.Now(),
			Kategori:   "REGULER",
			Jenis:      "MASUK",
			Nominal:    nota.TotalBayar, // Masukkan nilai akhir nota
			Keterangan: fmt.Sprintf("Pelunasan Reguler - %s (Toko: %s) [DIPULIHKAN]", nota.NoNota, nota.NamaTokoSnapshot),
			NoNotaRef:  nota.NoNota,
			CreatedBy:  adminID,
		})
	}

	tx.Commit()
	return c.JSON(fiber.Map{"message": "Nota berhasil dipulihkan!"})
}

// RIWAYAT NOTA
func GetNotas(c *fiber.Ctx) error {
	var notas []models.Nota
	if err := DB.Preload("Toko").Preload("Details").Preload("Details.Barang").Order("id desc").Find(&notas).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	} // Gunakan "id desc" agar nota yang baru dibuat muncul paling atas
	return c.JSON(notas)
}

func GetNotaByID(c *fiber.Ctx) error {
	id := c.Params("id")
	var nota models.Nota
	if err := DB.Preload("Toko").Preload("Details", func(db *gorm.DB) *gorm.DB {
		return db.Order("nota_details.id ASC")
	}).First(&nota, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Nota tidak ditemukan"})
	}
	return c.JSON(nota)
}

// DASHBOARD KUNJUNGAN SALES
func GetDashboardSales(c *fiber.Ctx) error {
	adminID := c.Locals("admin_id").(uint)
	var notaAktif []models.Nota
	var notaTugas []models.Nota
	var poTugas []models.NotaPesanan

	// Nota Aktif: 8 jam terakhir, status bebas
	DB.Preload("Toko").Where("created_by = ? AND created_at >= ?", adminID, time.Now().Add(-8*time.Hour)).Order("id desc").Find(&notaAktif)

	// Tugas Khusus (Reguler) dari Superadmin
	DB.Preload("Toko").Where("assigned_to = ? AND (jumlah_retur = 0 OR updated_at > ?)", adminID, time.Now().Add(-12*time.Hour)).Order("id desc").Find(&notaTugas)

	// BARU: Tugas Khusus Pesanan (PO) dari Superadmin yang BELUM SELESAI
	DB.Where("assigned_to = ? AND status != 'DIAMBIL'", adminID).Order("id desc").Find(&poTugas)

	// Kirim semua tugas ke Vue
	return c.JSON(fiber.Map{"aktif": notaAktif, "tugas": notaTugas, "tugas_po": poTugas})
}

func GetKunjunganToko(c *fiber.Ctx) error { // Memeriksa tagihan Retur saat tiba di toko
	tokoID := c.Params("toko_id")
	var notaBelumRetur []models.Nota

	DB.Preload("Toko").Where("toko_id = ? AND status = 'KIRIM' AND jumlah_retur = 0 AND tanggal_kirim >= ?",
		tokoID, time.Now().AddDate(0, -1, 0)).Order("tanggal_kirim asc").Find(&notaBelumRetur)

	return c.JSON(notaBelumRetur)
}
