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

// NOTA PESANAN
func GetNextNotaPesananNumber(c *fiber.Ctx) error {
	tgl := c.Query("tanggal") // 2026-04-30
	tglStr := strings.ReplaceAll(tgl, "-", "")
	tokoID := c.Query("toko_id")

	if tokoID == "" {
		tokoID = "0" // 0 berarti PABRIK
	}

	var poTerakhir models.NotaPesanan
	query := DB.Unscoped().Order("id desc")

	if tokoID == "0" {
		query.Where("toko_id IS NULL").First(&poTerakhir)
	} else {
		query.Where("toko_id = ?", tokoID).First(&poTerakhir)
	}

	nextUrutan := 1
	if poTerakhir.NoNota != "" {
		parts := strings.Split(poTerakhir.NoNota, "-")
		if len(parts) > 1 {
			lastNumStr := parts[len(parts)-1]
			if lastNum, err := strconv.Atoi(lastNumStr); err == nil {
				nextUrutan = lastNum + 1
			}
		}
	} else {
		// Fallback jika belum ada nota sama sekali
		var count int64
		if tokoID == "0" {
			DB.Unscoped().Model(&models.NotaPesanan{}).Where("toko_id IS NULL").Count(&count)
		} else {
			DB.Unscoped().Model(&models.NotaPesanan{}).Where("toko_id = ?", tokoID).Count(&count)
		}
		nextUrutan = int(count) + 1
	}

	// Format: PO/20260430/0-0001 (Pabrik) atau PO/20260430/15-0001 (Mitra)
	noNota := fmt.Sprintf("PO/%s/%s-%04d", tglStr, tokoID, nextUrutan)

	return c.JSON(fiber.Map{"no_nota": noNota})
}

func CreateNotaPesanan(c *fiber.Ctx) error {
	var input struct {
		NoNota           string  `json:"no_nota"`
		NamaPemesan      string  `json:"nama_pemesan"`
		TanggalKirim     string  `json:"tanggal_kirim"`
		JenisPengambilan string  `json:"jenis_pengambilan"`
		TokoID           *uint   `json:"toko_id"`
		AssignedTo       uint    `json:"assigned_to"`
		Status           string  `json:"status"`
		IsLunas          bool    `json:"is_lunas"`
		Ongkir           float64 `json:"ongkir"`
		UangMuka         float64 `json:"uang_muka"`     // <--- BARU: Tangkap DP
		TotalVoucher     float64 `json:"total_voucher"` // <--- BARU: Tangkap Voucher
		Details          []struct {
			BarangID        *uint   `json:"barang_id"`
			NamaBarangBebas string  `json:"nama_barang_bebas"`
			Banyak          int     `json:"banyak"`
			HargaJual       float64 `json:"harga_jual"`
			ResepID         *uint   `json:"resep_id"`
			Gramasi         float64 `json:"gramasi"`
			KemasanDetail   []struct {
				BahanID   uint    `json:"bahan_id"`
				Kebutuhan float64 `json:"kebutuhan"`
			} `json:"kemasan_detail"`
		} `json:"details"`
	}

	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format data salah"})
	}

	tgl, _ := time.Parse("2006-01-02", input.TanggalKirim)
	adminID := c.Locals("admin_id").(uint)

	namaTokoSnapshot := "PABRIK"
	if input.JenisPengambilan == "MITRA" && input.TokoID != nil {
		var toko models.Toko
		if err := DB.First(&toko, *input.TokoID).Error; err == nil {
			namaTokoSnapshot = toko.NamaToko
		}
	}

	pesanan := models.NotaPesanan{
		NoNota:           input.NoNota,
		NamaPemesan:      input.NamaPemesan,
		TanggalKirim:     tgl,
		JenisPengambilan: input.JenisPengambilan,
		TokoID:           input.TokoID,
		NamaTokoSnapshot: namaTokoSnapshot,
		CreatedBy:        adminID,
		AssignedTo:       input.AssignedTo,
		Status:           input.Status,
		IsLunas:          input.IsLunas,
	}

	var totalBayar float64
	for _, d := range input.Details {
		subtotal := float64(d.Banyak) * d.HargaJual
		totalBayar += subtotal

		// --- LOGIKA KEMASAN BARU ---
		var kemasanArr []models.NotaPesananDetailKemasan
		for _, k := range d.KemasanDetail {
			kemasanArr = append(kemasanArr, models.NotaPesananDetailKemasan{
				BahanID:   k.BahanID,
				Kebutuhan: k.Kebutuhan,
			})
		}

		pesanan.Details = append(pesanan.Details, models.NotaPesananDetail{
			BarangID:        d.BarangID,
			NamaBarangBebas: d.NamaBarangBebas,
			Banyak:          d.Banyak,
			HargaJual:       d.HargaJual,
			Subtotal:        subtotal,
			ResepID:         d.ResepID,
			Gramasi:         d.Gramasi,
			KemasanDetail:   kemasanArr,
		})
	}

	// LOGIKA UANG RIIL PO
	pesanan.TotalBayar = totalBayar
	pesanan.Ongkir = input.Ongkir
	pesanan.UangMuka = input.UangMuka
	pesanan.TotalVoucher = input.TotalVoucher
	pesanan.SisaTagihan = totalBayar + input.Ongkir - input.UangMuka - input.TotalVoucher

	if err := DB.Create(&pesanan).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// ==========================================
	// FULL SYNC KAS: CREATE PO BARU
	// ==========================================
	var settingKas models.PengaturanSistem
	DB.Where("key = ?", "ENABLE_KAS_SYNC").First(&settingKas)

	if settingKas.Value == "true" {

		// 1. Catat DP Jika Ada
		if pesanan.UangMuka > 0 {
			DB.Create(&models.TransaksiKas{
				Tanggal:    time.Now(),
				Kategori:   "PESANAN",
				Jenis:      "MASUK",
				Nominal:    pesanan.UangMuka,
				Keterangan: fmt.Sprintf("DP Pesanan - %s (Pemesan: %s)", pesanan.NoNota, pesanan.NamaPemesan),
				NoNotaRef:  pesanan.NoNota,
				CreatedBy:  adminID,
			})
		}

		// 2. Catat Pelunasan Sisa PO jika langsung dilunasi
		if pesanan.IsLunas && pesanan.SisaTagihan > 0 {
			DB.Create(&models.TransaksiKas{
				Tanggal:    time.Now(),
				Kategori:   "PESANAN",
				Jenis:      "MASUK",
				Nominal:    pesanan.SisaTagihan,
				Keterangan: fmt.Sprintf("Pelunasan Sisa PO - %s (Pemesan: %s)", pesanan.NoNota, pesanan.NamaPemesan),
				NoNotaRef:  pesanan.NoNota,
				CreatedBy:  adminID,
			})
		}
	}

	return c.JSON(fiber.Map{"message": "Pesanan berhasil dibuat!", "id": pesanan.ID})
}

// UPDATE PO
func UpdateNotaPesanan(c *fiber.Ctx) error {
	id := c.Params("id")
	var input struct {
		NamaPemesan      string  `json:"nama_pemesan"`
		TanggalKirim     string  `json:"tanggal_kirim"`
		JenisPengambilan string  `json:"jenis_pengambilan"`
		TokoID           *uint   `json:"toko_id"`
		AssignedTo       uint    `json:"assigned_to"`
		Status           string  `json:"status"`
		IsLunas          bool    `json:"is_lunas"`
		Ongkir           float64 `json:"ongkir"`
		UangMuka         float64 `json:"uang_muka"`     // <--- BARU: Tangkap DP
		TotalVoucher     float64 `json:"total_voucher"` // <--- BARU: Tangkap Voucher
		Details          []struct {
			BarangID        *uint   `json:"barang_id"`
			NamaBarangBebas string  `json:"nama_barang_bebas"`
			Banyak          int     `json:"banyak"`
			HargaJual       float64 `json:"harga_jual"`
			ResepID         *uint   `json:"resep_id"`
			Gramasi         float64 `json:"gramasi"`
			KemasanDetail   []struct {
				BahanID   uint    `json:"bahan_id"`
				Kebutuhan float64 `json:"kebutuhan"`
			} `json:"kemasan_detail"`
		} `json:"details"`
	}

	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format data salah"})
	}

	tgl, _ := time.Parse("2006-01-02", input.TanggalKirim)

	// --- MULAI BLOK REFUND STOK (TAMBAHKAN INI) ---
	var detailLama []models.NotaPesananDetail
	// Tarik data detail lama beserta kemasannya
	DB.Preload("KemasanDetail").Where("nota_pesanan_id = ?", id).Find(&detailLama)

	for _, d := range detailLama {
		// Jika statusnya sudah pernah dipotong oleh Tutup Buku, kembalikan stoknya dulu
		if d.IsKemasanTerpotong {
			for _, k := range d.KemasanDetail {
				totalBalik := float64(d.Banyak) * k.Kebutuhan
				DB.Model(&models.Bahan{}).Where("id = ?", k.BahanID).
					Update("stok", gorm.Expr("stok + ?", totalBalik))
			}
		}
	}

	// 1. HAPUS KEMASAN DULU (ANAKNYA) AGAR TIDAK DIBLOKIR FOREIGN KEY
	DB.Exec("DELETE FROM nota_pesanan_detail_kemasans WHERE nota_pesanan_detail_id IN (SELECT id FROM nota_pesanan_details WHERE nota_pesanan_id = ?)", id)

	// 2. BARU HAPUS DETAIL LAMA (INDUKNYA)
	DB.Where("nota_pesanan_id = ?", id).Delete(&models.NotaPesananDetail{})

	var totalBayar float64
	var newDetails []models.NotaPesananDetail

	for _, d := range input.Details {
		sub := float64(d.Banyak) * d.HargaJual
		totalBayar += sub
		parsedID, _ := strconv.Atoi(id)

		// --- LOGIKA KEMASAN BARU ---
		var kemasanArr []models.NotaPesananDetailKemasan
		for _, k := range d.KemasanDetail {
			kemasanArr = append(kemasanArr, models.NotaPesananDetailKemasan{
				BahanID:   k.BahanID,
				Kebutuhan: k.Kebutuhan,
			})
		}

		newDetails = append(newDetails, models.NotaPesananDetail{
			NotaPesananID:   uint(parsedID),
			BarangID:        d.BarangID,
			NamaBarangBebas: d.NamaBarangBebas,
			Banyak:          d.Banyak,
			HargaJual:       d.HargaJual,
			Subtotal:        sub,
			ResepID:         d.ResepID,
			Gramasi:         d.Gramasi,
			KemasanDetail:   kemasanArr,
		})
	}

	DB.Create(&newDetails)

	// Update Header
	namaTokoSnap := "PABRIK"
	if input.JenisPengambilan == "MITRA" && input.TokoID != nil {
		var t models.Toko
		DB.First(&t, *input.TokoID)
		namaTokoSnap = t.NamaToko
	}

	// HITUNG ULANG SISA TAGIHAN SAAT DI-UPDATE
	sisaTagihan := totalBayar + input.Ongkir - input.UangMuka - input.TotalVoucher

	DB.Model(&models.NotaPesanan{}).Where("id = ?", id).Updates(map[string]interface{}{
		"nama_pemesan":       input.NamaPemesan,
		"tanggal_kirim":      tgl,
		"jenis_pengambilan":  input.JenisPengambilan,
		"toko_id":            input.TokoID,
		"nama_toko_snapshot": namaTokoSnap,
		"assigned_to":        input.AssignedTo,
		"status":             input.Status,
		"is_lunas":           input.IsLunas,
		"total_bayar":        totalBayar,
		"ongkir":             input.Ongkir,
		"uang_muka":          input.UangMuka,     // <--- UPDATE DP
		"total_voucher":      input.TotalVoucher, // <--- UPDATE VOUCHER
		"sisa_tagihan":       sisaTagihan,        // <--- UPDATE SISA
	})

	// ==========================================
	// FULL SYNC KAS: UPDATE PO
	// ==========================================
	var settingKas models.PengaturanSistem
	DB.Where("key = ?", "ENABLE_KAS_SYNC").First(&settingKas)

	if settingKas.Value == "true" {
		var poLama models.NotaPesanan
		DB.First(&poLama, id)
		adminID := c.Locals("admin_id").(uint)

		// 1. SINKRONISASI DP PESANAN
		var kasDP models.TransaksiKas
		errDP := DB.Where("no_nota_ref = ? AND keterangan LIKE 'DP Pesanan%'", poLama.NoNota).First(&kasDP).Error

		if input.UangMuka > 0 {
			ketDP := fmt.Sprintf("DP Pesanan - %s (Pemesan: %s)", poLama.NoNota, input.NamaPemesan)
			if errDP == nil {
				DB.Model(&kasDP).Updates(map[string]interface{}{"nominal": input.UangMuka, "keterangan": ketDP})
			} else {
				DB.Create(&models.TransaksiKas{
					Tanggal: time.Now(), Kategori: "PESANAN", Jenis: "MASUK",
					Nominal: input.UangMuka, Keterangan: ketDP, NoNotaRef: poLama.NoNota, CreatedBy: adminID,
				})
			}
		} else { // Jika DP di-nol-kan, hapus kas DP
			if errDP == nil {
				DB.Unscoped().Delete(&kasDP)
			}
		}

		// 2. SINKRONISASI PELUNASAN SISA TAGIHAN
		var kasSisa models.TransaksiKas
		errSisa := DB.Where("no_nota_ref = ? AND keterangan LIKE 'Pelunasan Sisa PO%'", poLama.NoNota).First(&kasSisa).Error

		if input.IsLunas && sisaTagihan > 0 {
			ketSisa := fmt.Sprintf("Pelunasan Sisa PO - %s (Pemesan: %s)", poLama.NoNota, input.NamaPemesan)
			if errSisa == nil {
				DB.Model(&kasSisa).Updates(map[string]interface{}{"nominal": sisaTagihan, "keterangan": ketSisa})
			} else {
				DB.Create(&models.TransaksiKas{
					Tanggal: time.Now(), Kategori: "PESANAN", Jenis: "MASUK",
					Nominal: sisaTagihan, Keterangan: ketSisa, NoNotaRef: poLama.NoNota, CreatedBy: adminID,
				})
			}
		} else { // Jika Batal Lunas (atau Sisa Tagihan jadi 0), hapus kas pelunasan
			if errSisa == nil {
				DB.Unscoped().Delete(&kasSisa)
			}
		}
	}

	return c.JSON(fiber.Map{"message": "Pesanan diupdate!"})
}

// Batalkan Pesanan PO (Soft Cancel & Tarik Kas)
func BatalkanPesanan(c *fiber.Ctx) error {
	id := c.Params("id")
	tx := DB.Begin()

	var pesanan models.NotaPesanan
	if err := tx.First(&pesanan, id).Error; err != nil {
		tx.Rollback()
		return c.Status(404).JSON(fiber.Map{"error": "Pesanan tidak ditemukan"})
	}

	// ==============================================================
	// BARU: REFUND KEMASAN KUSTOM JIKA SUDAH TERPOTONG TUTUP BUKU
	// ==============================================================
	var details []models.NotaPesananDetail
	// Tarik detail pesanan kustom yang gemboknya SUDAH TERKUNCI (true)
	tx.Preload("KemasanDetail").Where("nota_pesanan_id = ? AND is_kemasan_terpotong = ?", id, true).Find(&details)

	for _, pk := range details {
		// Pastikan ini barang kustom (BarangID nil) dan punya kemasan
		if pk.BarangID == nil && len(pk.KemasanDetail) > 0 {
			for _, k := range pk.KemasanDetail {
				totalRefund := float64(pk.Banyak) * k.Kebutuhan
				// Kembalikan (Refund) stok kardus ke master bahan
				tx.Model(&models.Bahan{}).Where("id = ?", k.BahanID).
					Update("stok", gorm.Expr("stok + ?", totalRefund))
			}
		}

		// BUKA GEMBOKNYA: Agar statusnya kembali sinkron
		tx.Model(&models.NotaPesananDetail{}).Where("id = ?", pk.ID).Update("is_kemasan_terpotong", false)
	}
	// ==============================================================

	// 1. Ubah status
	if err := tx.Model(&pesanan).Update("status", "DIBATALKAN").Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 2. Tarik uang DP dan Pelunasan dari Brankas
	tx.Unscoped().Where("no_nota_ref = ?", pesanan.NoNota).Delete(&models.TransaksiKas{})

	tx.Commit()
	return c.JSON(fiber.Map{"message": "Pesanan berhasil dibatalkan dan Kas ditarik kembali!"})
}

// PULIHKAN NOTA PESANAN (PO)
func PulihkanPesanan(c *fiber.Ctx) error {
	id := c.Params("id")
	tx := DB.Begin()

	var pesanan models.NotaPesanan
	if err := tx.First(&pesanan, id).Error; err != nil {
		tx.Rollback()
		return c.Status(404).JSON(fiber.Map{"error": "Pesanan tidak ditemukan"})
	}

	// 1. Kembalikan status
	if err := tx.Model(&pesanan).Update("status", "MENUNGGU").Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 2. Kembalikan Kas (DP & Pelunasan)
	var settingKas models.PengaturanSistem
	tx.Where("key = ?", "ENABLE_KAS_SYNC").First(&settingKas)

	if settingKas.Value == "true" {
		adminID := c.Locals("admin_id").(uint)

		if pesanan.UangMuka > 0 {
			tx.Create(&models.TransaksiKas{
				Tanggal:    time.Now(),
				Kategori:   "PESANAN",
				Jenis:      "MASUK",
				Nominal:    pesanan.UangMuka,
				Keterangan: fmt.Sprintf("DP Pesanan - %s (Pemesan: %s) [DIPULIHKAN]", pesanan.NoNota, pesanan.NamaPemesan),
				NoNotaRef:  pesanan.NoNota,
				CreatedBy:  adminID,
			})
		}

		if pesanan.IsLunas && pesanan.SisaTagihan > 0 {
			tx.Create(&models.TransaksiKas{
				Tanggal:    time.Now(),
				Kategori:   "PESANAN",
				Jenis:      "MASUK",
				Nominal:    pesanan.SisaTagihan,
				Keterangan: fmt.Sprintf("Pelunasan Sisa PO - %s (Pemesan: %s) [DIPULIHKAN]", pesanan.NoNota, pesanan.NamaPemesan),
				NoNotaRef:  pesanan.NoNota,
				CreatedBy:  adminID,
			})
		}
	}

	tx.Commit()
	return c.JSON(fiber.Map{"message": "Pesanan berhasil dipulihkan!"})
}

// 1. Get Semua Riwayat Pesanan
func GetRiwayatPesanan(c *fiber.Ctx) error {
	var pesanan []models.NotaPesanan
	// Urutkan dari yang terbaru, hapus Where("riwayat") yang error
	if err := DB.Preload("Toko").Preload("Details").Order("id desc").Find(&pesanan).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(pesanan)
}

// GET PO BY ID
func GetNotaPesananByID(c *fiber.Ctx) error {
	id := c.Params("id")
	var pesanan models.NotaPesanan

	// UBAH BARIS INI AGAR GOLANG MENGIRIM DATA KEMASAN SAAT DI-EDIT:
	if err := DB.Preload("Toko").Preload("Details").Preload("Details.Barang").Preload("Details.KemasanDetail").First(&pesanan, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Pesanan tidak ditemukan"})
	}

	return c.JSON(pesanan)
}

func GetCatatanPesanan(c *fiber.Ctx) error {
	tgl := c.Query("tanggal") // Cukup kirim 1 tanggal (hari H)

	var results []struct {
		NamaBarangBebas  string  `json:"nama_barang_bebas"`
		NamaTokoSnapshot string  `json:"nama_toko"`
		JenisPengambilan string  `json:"jenis_pengambilan"`
		TotalBanyak      int     `json:"total_banyak"`
		TotalRupiah      float64 `json:"total_rupiah"` // <--- BARU: Tangkap jumlah uang
	}

	// Query rekap berdasarkan hari H pengiriman pesanan
	err := DB.Table("nota_pesanan_details").
		Select("nota_pesanan_details.nama_barang_bebas, nota_pesanans.nama_toko_snapshot, nota_pesanans.jenis_pengambilan, SUM(nota_pesanan_details.banyak) as total_banyak, SUM(nota_pesanan_details.subtotal) as total_rupiah"). // <--- BARU: Tarik Subtotal
		Joins("join nota_pesanans on nota_pesanans.id = nota_pesanan_details.nota_pesanan_id").
		Where("DATE(nota_pesanans.tanggal_kirim) = ?", tgl).
		Group("nota_pesanan_details.nama_barang_bebas, nota_pesanans.nama_toko_snapshot, nota_pesanans.jenis_pengambilan").
		Scan(&results).Error

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(results)
}
