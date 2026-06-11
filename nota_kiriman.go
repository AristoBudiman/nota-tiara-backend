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
//
// @Summary Dapatkan nomor nota berikutnya
// @Description Meng-generate nomor nota urut yang pintar berdasarkan tanggal dan ID toko (Cth: NT/20260427/15-0017)
// @Tags 08. Nota Reguler
// @Accept json
// @Produce json
// @Param toko_id query string false "Filter berdasarkan ID Toko (Jika 0 = Pabrik)"
// @Param tanggal query string false "Format: YYYY-MM-DD (Default: Hari ini)"
// @Success 200 {object} models.NextNotaResponse
// @Router /notas/next-number [get]
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

// @Summary Buat Nota Reguler baru
// @Description Memasukkan data nota kiriman harian beserta detail array barangnya ke database. Akan memicu trigger sinkronisasi brankas (Kas) jika nota ditandai lunas.
// @Tags 08. Nota Reguler
// @Accept json
// @Produce json
// @Param payload body models.NotaInput true "Data lengkap nota dan detail barang"
// @Success 200 {object} models.MessageResponse "Pesan: Nota berhasil disimpan!"
// @Failure 400 {object} map[string]interface{} "Format data JSON tidak valid"
// @Failure 500 {object} map[string]interface{} "Kesalahan eksekusi database / kas"
// @Router /notas [post]
func CreateNota(c *fiber.Ctx) error {
	var input struct {
		NoNota       string  `json:"no_nota"`
		TokoID       uint    `json:"toko_id"`
		TanggalKirim string  `json:"tanggal_kirim"`
		AssignedTo   uint    `json:"assigned_to"`
		Status       string  `json:"status"`
		IsLunas      bool    `json:"is_lunas"`
		TotalDiskon  float64 `json:"total_diskon"`
		TotalVoucher float64 `json:"total_voucher"`
		Details      []struct {
			BarangID    uint `json:"barang_id"`
			BanyakKirim int  `json:"banyak_kirim"`
		} `json:"details"`
	}

	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format data salah"})
	}

	if input.TotalDiskon < 0 || input.TotalVoucher < 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Diskon atau voucher tidak boleh minus."})
	}
	for _, d := range input.Details {
		if d.BanyakKirim <= 0 {
			return c.Status(400).JSON(fiber.Map{"error": "Kuantitas barang kirim harus lebih besar dari 0."})
		}
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
		TotalDiskon:      input.TotalDiskon,
		TotalVoucher:     input.TotalVoucher,
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
	nota.TotalBayar = totalKirim - input.TotalDiskon - input.TotalVoucher

	tx := DB.Begin()
	if err := tx.Create(&nota).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// ==========================================
	// FULL SYNC KAS: CREATE NOTA LUNAS
	// ==========================================
	var settingKas models.PengaturanSistem
	tx.Where("key = ?", "ENABLE_KAS_SYNC").First(&settingKas)
	if settingKas.Value == "true" {
		if input.IsLunas {
			tx.Create(&models.TransaksiKas{
				Tanggal:    wib(),
				Kategori:   "REGULER",
				Jenis:      "MASUK",
				Nominal:    totalKirim,
				Keterangan: fmt.Sprintf("Pelunasan Reguler - %s (Toko: %s)", nota.NoNota, toko.NamaToko),
				NoNotaRef:  nota.NoNota,
				CreatedBy:  adminID,
			})
			TambahSaldoKas(tx, totalKirim)
		}
	}
	tx.Commit()
	return c.JSON(fiber.Map{"message": "Nota berhasil dibuat!", "id": nota.ID})
}

// @Summary Ubah data Nota Reguler
// @Description Mengubah data nota kiriman historis berdasarkan ID. Endpoint ini sangat krusial karena otomatis menghitung selisih uang jika status lunas/tidak lunas berubah.
// @Tags 08. Nota Reguler
// @Accept json
// @Produce json
// @Param id path int true "ID Nota"
// @Param payload body models.NotaInput true "Data nota yang akan direvisi"
// @Success 200 {object} models.MessageResponse "Pesan: Nota berhasil diupdate!"
// @Failure 400 {object} map[string]interface{} "Format data JSON tidak valid"
// @Failure 404 {object} map[string]interface{} "Nota tidak ditemukan"
// @Failure 500 {object} map[string]interface{} "Kesalahan eksekusi database / kas"
// @Router /notas/{id} [put]
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

	if input.TotalDiskon < 0 || input.TotalVoucher < 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Diskon atau voucher tidak boleh minus."})
	}
	for _, d := range input.Details {
		if d.BanyakKirim <= 0 || d.BanyakRetur < 0 || d.HargaJual < 0 {
			return c.Status(400).JSON(fiber.Map{"error": "Detail barang tidak valid. Kuantitas kirim harus > 0, retur dan harga tidak boleh minus."})
		}
	}

	tglBaru, errDate := time.Parse("2006-01-02", input.TanggalKirim)
	if errDate != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format tanggal tidak valid"})
	}

	// CLEANUP: Hapus detail siluman/duplikat yang tidak dikirim oleh frontend
	var sentDetailIDs []uint
	for _, d := range input.Details {
		if d.ID != 0 {
			sentDetailIDs = append(sentDetailIDs, d.ID)
		}
	}
	if len(sentDetailIDs) > 0 {
		DB.Where("nota_id = ? AND id NOT IN ?", id, sentDetailIDs).Delete(&models.NotaDetail{})
	} else {
		DB.Where("nota_id = ?", id).Delete(&models.NotaDetail{})
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

		// Ganti .First() menjadi .Find()
		result := DB.Where("no_nota_ref = ? AND kategori = 'REGULER'", notaLama.NoNota).Find(&kasReguler)

		if input.IsLunas {
			ket := fmt.Sprintf("Pelunasan Reguler - %s (Toko: %s)", notaLama.NoNota, notaLama.NamaTokoSnapshot)

			// Ganti pengecekan errKas menjadi pengecekan jumlah baris
			if result.RowsAffected > 0 {
				selisih := totalBayarAkhir - kasReguler.Nominal
				// Sudah ada kasnya, UPDATE nominalnya (Bisa jadi ada tambahan retur/diskon)
				DB.Model(&kasReguler).Updates(map[string]interface{}{
					"nominal":    totalBayarAkhir,
					"keterangan": ket,
				})
				TambahSaldoKas(DB, selisih)
			} else {
				// Belum ada, CREATE kas masuk
				DB.Create(&models.TransaksiKas{
					Tanggal:    wib(),
					Kategori:   "REGULER",
					Jenis:      "MASUK",
					Nominal:    totalBayarAkhir,
					Keterangan: ket,
					NoNotaRef:  notaLama.NoNota,
					CreatedBy:  adminID,
				})
				TambahSaldoKas(DB, totalBayarAkhir)
			}
		} else {
			// Jika TIDAK LUNAS (atau Batal Lunas), HAPUS KAS JIKA ADA!
			if result.RowsAffected > 0 { // Ganti pengecekan errKas di sini juga
				DB.Unscoped().Delete(&kasReguler)
				KurangiSaldoKas(DB, kasReguler.Nominal)
			}
		}
	}

	return c.JSON(fiber.Map{"message": "Nota dan Qty Kirim berhasil diupdate!"})
}

// Batalkan Nota Reguler (Soft Delete & Tarik Kas)
//
// @Summary Batalkan Nota Reguler (Rollback)
// @Description Mengubah status nota menjadi 'DIBATALKAN'. Jika sebelumnya lunas, sistem otomatis akan menarik uang dari brankas/kas untuk menjaga integritas pembukuan.
// @Tags 08. Nota Reguler
// @Accept json
// @Produce json
// @Param id path int true "ID Nota"
// @Success 200 {object} models.MessageResponse "Pesan: Nota berhasil dibatalkan"
// @Failure 404 {object} map[string]interface{} "Nota tidak ditemukan"
// @Failure 500 {object} map[string]interface{} "Kesalahan saat membatalkan transaksi kas"
// @Router /notas/{id}/batal [put]
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
	var kasList []models.TransaksiKas
	tx.Where("no_nota_ref = ?", nota.NoNota).Find(&kasList)
	for _, k := range kasList {
		KurangiSaldoKas(tx, k.Nominal)
	}
	tx.Unscoped().Where("no_nota_ref = ?", nota.NoNota).Delete(&models.TransaksiKas{})

	tx.Commit()
	return c.JSON(fiber.Map{"message": "Nota berhasil dibatalkan dan Kas ditarik kembali!"})
}

// PULIHKAN NOTA REGULER
//
// @Summary Pulihkan Nota Reguler
// @Description Mengembalikan status nota dari 'DIBATALKAN' menjadi 'KIRIM'. Menginjeksi ulang uang ke brankas jika nota tercatat lunas.
// @Tags 08. Nota Reguler
// @Accept json
// @Produce json
// @Param id path int true "ID Nota"
// @Success 200 {object} models.MessageResponse "Pesan: Nota berhasil dipulihkan"
// @Failure 404 {object} map[string]interface{} "Nota tidak ditemukan"
// @Failure 500 {object} map[string]interface{} "Kesalahan server"
// @Router /notas/{id}/pulihkan [put]
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
			Tanggal:    wib(),
			Kategori:   "REGULER",
			Jenis:      "MASUK",
			Nominal:    nota.TotalBayar, // Masukkan nilai akhir nota
			Keterangan: fmt.Sprintf("Pelunasan Reguler - %s (Toko: %s) [DIPULIHKAN]", nota.NoNota, nota.NamaTokoSnapshot),
			NoNotaRef:  nota.NoNota,
			CreatedBy:  adminID,
		})
		TambahSaldoKas(tx, nota.TotalBayar)
	}

	tx.Commit()
	return c.JSON(fiber.Map{"message": "Nota berhasil dipulihkan!"})
}

// RIWAYAT NOTA
//
// @Summary Ambil Semua Riwayat Nota
// @Description Menampilkan daftar keseluruhan riwayat nota kiriman dari yang terbaru (descending).
// @Tags 08. Nota Reguler
// @Accept json
// @Produce json
// @Success 200 {array} models.Nota
// @Failure 500 {object} map[string]interface{} "Kesalahan server"
// @Router /notas [get]
func GetNotas(c *fiber.Ctx) error {
	var notas []models.Nota

	// Tangkap parameter tanggal dari Vue
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	// Siapkan pondasi query
	query := DB.Preload("Toko").Preload("Details").Preload("Details.Barang").Order("id desc")

	// Jika ada filter tanggal, tambahkan klausa WHERE
	if startDate != "" && endDate != "" {
		query = query.Where("tanggal_kirim >= ? AND tanggal_kirim <= ?", startDate+" 00:00:00", endDate+" 23:59:59")
	}

	if err := query.Find(&notas).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(notas)
}

// @Summary Ambil Detail Nota (Berdasarkan ID)
// @Description Menampilkan data spesifik sebuah nota beserta hierarki relasinya (Detail Nota, Barang terkait, dan Toko).
// @Tags 08. Nota Reguler
// @Accept json
// @Produce json
// @Param id path int true "ID Nota"
// @Success 200 {object} models.Nota
// @Failure 404 {object} map[string]interface{} "Nota tidak ditemukan"
// @Router /notas/{id} [get]
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
//
// @Summary Muat Dashboard Sales / Kurir
// @Description Menarik 3 himpunan tugas sekaligus untuk user sales yang login: Nota aktif 8 jam terakhir, Tugas membereskan retur (Reguler), dan Tugas mengantar pesanan khusus (PO).
// @Tags 16. Distribusi Lapangan
// @Accept json
// @Produce json
// @Success 200 {object} models.DashboardSalesResponse
// @Router /sales/dashboard [get]
func GetDashboardSales(c *fiber.Ctx) error {
	adminID := c.Locals("admin_id").(uint)
	var notaAktif []models.Nota
	var notaTugas []models.Nota
	var poTugas []models.NotaPesanan

	// Nota Aktif: 8 jam terakhir, status bebas
	DB.Preload("Toko").Where("created_by = ? AND created_at >= ?", adminID, wib().Add(-8*time.Hour)).Order("id desc").Find(&notaAktif)

	// Tugas Khusus (Reguler) dari Superadmin
	DB.Preload("Toko").Where("assigned_to = ? AND (jumlah_retur = 0 OR updated_at > ?)", adminID, wib().Add(-12*time.Hour)).Order("id desc").Find(&notaTugas)

	// BARU: Tugas Khusus Pesanan (PO) dari Superadmin yang BELUM SELESAI
	DB.Where("assigned_to = ? AND status != 'DIAMBIL'", adminID).Order("id desc").Find(&poTugas)

	// Kirim semua tugas ke Vue
	return c.JSON(fiber.Map{"aktif": notaAktif, "tugas": notaTugas, "tugas_po": poTugas})
}

// @Summary Sidak Dosa Retur Toko
// @Description Mengecek apakah sebuah toko memiliki nota berstatus 'KIRIM' namun belum diselesaikan proses returnya. Digunakan untuk memblokir pembuatan nota baru oleh sales.
// @Tags 16. Distribusi Lapangan
// @Accept json
// @Produce json
// @Param toko_id path int true "ID Toko"
// @Success 200 {array} models.Nota
// @Router /sales/kunjungan/{toko_id} [get]
func GetKunjunganToko(c *fiber.Ctx) error { // Memeriksa tagihan Retur saat tiba di toko
	tokoID := c.Params("toko_id")
	var notaBelumRetur []models.Nota

	DB.Preload("Toko").Where("toko_id = ? AND status = 'KIRIM' AND jumlah_retur = 0 AND tanggal_kirim >= ?",
		tokoID, wib().AddDate(0, -1, 0)).Order("tanggal_kirim asc").Find(&notaBelumRetur)

	return c.JSON(notaBelumRetur)
}
