package main

import (
	"backend/models"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// GetPembelianBahan godoc
// @Summary Riwayat Pembelian Bahan Baku
// @Description Menarik riwayat belanja bahan fisik.
// @Tags 11. Operasional Inventory
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param start query string false "Tanggal Mulai (YYYY-MM-DD)"
// @Param end query string false "Tanggal Akhir (YYYY-MM-DD)"
// @Success 200 {array} models.PembelianBahan "Berhasil ditarik"
// @Router /api/pembelian [get]
func GetPembelianBahan(c *fiber.Ctx) error {
	start := c.Query("start")
	end := c.Query("end")
	status := c.Query("status")

	var beli []models.NotaPembelian

	// Tarik Induk beserta detail anak dan relasi nama bahannya
	query := DB.Preload("Details.Bahan", func(db *gorm.DB) *gorm.DB {
		return db.Unscoped()
	})

	if start != "" && end != "" {
		query = query.Where("tanggal >= ? AND tanggal <= ?", start, end)
	}

	if status == "sampah" {
		query = query.Unscoped().Where("deleted_at IS NOT NULL")
	}

	if err := query.Order("tanggal desc, id desc").Find(&beli).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(beli)
}

// PEMBELIAN BAHAN (UPDATE OTOMATIS)
//
// CreatePembelianBahan godoc
// @Summary Catat Pembelian Bahan (Otomatis Tambah Stok)
// @Description Mencatat belanja bahan, otomatis menambah stok gudang, menimpa harga beli terbaru, dan memotong kas (jika lunas).
// @Tags 11. Operasional Inventory
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param payload body models.PembelianBahanInput true "Data pembelian lengkap"
// @Success 200 {object} models.MessageResponse "Pembelian berhasil dicatat"
// @Failure 500 {object} models.ErrorResponse "Gagal potong kas/stok"
// @Router /api/pembelian [post]
func CreatePembelianBahan(c *fiber.Ctx) error {
	var input struct {
		Tanggal    string `json:"tanggal"`
		Keterangan string `json:"keterangan"`
		IsLunas    bool   `json:"is_lunas"`
		Details    []struct {
			BahanID         uint    `json:"bahan_id"`
			Qty             float64 `json:"qty"`
			HargaBeliSatuan float64 `json:"harga_beli_satuan"`
			Subtotal        float64 `json:"subtotal"`
		} `json:"details"`
	}

	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format data salah"})
	}

	for _, d := range input.Details {
		if d.Qty <= 0 || d.HargaBeliSatuan < 0 {
			return c.Status(400).JSON(fiber.Map{"error": "Kuantitas atau harga beli tidak valid. Tidak boleh minus atau nol."})
		}
	}

	tgl, _ := time.Parse("2006-01-02", input.Tanggal)
	tx := DB.Begin()

	var grandTotal float64

	// 1. Siapkan Induk Nota Pembelian
	pembelian := models.NotaPembelian{
		Tanggal:    tgl,
		Keterangan: input.Keterangan,
		IsLunas:    input.IsLunas,
	}

	// 2. Loop rincian bahan belanjaan
	for _, d := range input.Details {
		grandTotal += d.Subtotal

		// Masukkan ke detail nota
		pembelian.Details = append(pembelian.Details, models.NotaPembelianDetail{
			BahanID:         d.BahanID,
			Qty:             d.Qty,
			HargaBeliSatuan: d.HargaBeliSatuan,
			Subtotal:        d.Subtotal,
		})

		// 3. Langsung Update Stok & Harga HPP per bahan
		if err := tx.Model(&models.Bahan{}).Where("id = ?", d.BahanID).Updates(map[string]interface{}{
			"stok":           gorm.Expr("stok + ?", d.Qty),
			"harga_saat_ini": d.HargaBeliSatuan,
		}).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": "Gagal update stok bahan"})
		}
	}

	pembelian.TotalBiaya = grandTotal

	// Simpan Nota dan Detailnya ke DB sekaligus
	if err := tx.Create(&pembelian).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "Gagal mencatat nota pembelian"})
	}

	// 4. FULL SYNC KAS: HANYA 1 TRANSAKSI UNTUK 1 STRUK (Jika Lunas)
	var settingKas models.PengaturanSistem
	DB.Where("key = ?", "ENABLE_KAS_SYNC").First(&settingKas)

	if settingKas.Value == "true" && input.IsLunas {
		adminID := c.Locals("admin_id").(uint)

		ketKas := fmt.Sprintf("Belanja Bahan Baku (Nota #%d) - %d Macam Item. Keterangan: %s", pembelian.ID, len(input.Details), input.Keterangan)

		if err := tx.Create(&models.TransaksiKas{
			Tanggal:    wib(),
			Kategori:   "BAHAN",
			Jenis:      "KELUAR",
			Nominal:    grandTotal,
			Keterangan: ketKas,
			NoNotaRef:  fmt.Sprintf("BELI-%d", pembelian.ID),
			CreatedBy:  adminID,
		}).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": "Gagal memotong uang kas"})
		}
		KurangiSaldoKas(tx, grandTotal)
	}

	tx.Commit()
	return c.JSON(fiber.Map{"message": "Struk belanja berhasil dicatat, stok bertambah!"})
}

// FUNGSI BARU: UBAH STATUS BAYAR (LUNAS <-> HUTANG)
//
// UpdateStatusPembelian godoc
// @Summary Update Status Pembayaran Belanja (Hutang/Lunas)
// @Description Mengubah status lunas pembelian bahan. Otomatis menarik uang atau memotong kas secara sinkron.
// @Tags 11. Operasional Inventory
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "ID Pembelian"
// @Param payload body models.StatusPembelianInput true "Saklar Lunas"
// @Success 200 {object} models.MessageResponse "Status diupdate"
// @Router /api/pembelian/{id}/status [put]
func UpdateStatusPembelian(c *fiber.Ctx) error {
	id := c.Params("id")
	var input struct {
		IsLunas bool `json:"is_lunas"`
	}

	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format data salah"})
	}

	tx := DB.Begin()
	var p models.NotaPembelian
	if err := tx.First(&p, id).Error; err != nil {
		tx.Rollback()
		return c.Status(404).JSON(fiber.Map{"error": "Nota pembelian tidak ditemukan"})
	}

	if err := tx.Model(&p).Update("is_lunas", input.IsLunas).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "Gagal update status"})
	}

	var settingKas models.PengaturanSistem
	DB.Where("key = ?", "ENABLE_KAS_SYNC").First(&settingKas)

	if settingKas.Value == "true" {
		adminID := c.Locals("admin_id").(uint)
		noNotaRefBeli := fmt.Sprintf("BELI-%d", p.ID)

		if input.IsLunas {
			ketKas := fmt.Sprintf("Pelunasan Nota Belanja #%d - %s", p.ID, p.Keterangan)
			var existingKas models.TransaksiKas

			// Gunakan Find() agar GORM tidak teriak error saat data memang belum ada
			result := tx.Where("no_nota_ref = ?", noNotaRefBeli).Limit(1).Find(&existingKas)

			if result.RowsAffected == 0 {
				tx.Create(&models.TransaksiKas{
					Tanggal:    wib(),
					Kategori:   "BAHAN",
					Jenis:      "KELUAR",
					Nominal:    p.TotalBiaya,
					Keterangan: ketKas,
					NoNotaRef:  noNotaRefBeli,
					CreatedBy:  adminID,
				})
				KurangiSaldoKas(tx, p.TotalBiaya)
			}
		} else {
			var kas models.TransaksiKas
			res := tx.Where("no_nota_ref = ?", noNotaRefBeli).First(&kas)
			if res.Error == nil {
				TambahSaldoKas(tx, kas.Nominal)
				tx.Unscoped().Delete(&kas)
			}
		}
	}

	tx.Commit()
	return c.JSON(fiber.Map{"message": "Status pembayaran berhasil diupdate!"})
}

// FUNGSI BARU: BATALKAN PEMBELIAN & REFUND STOK/KAS
//
// DeletePembelianBahan godoc
// @Summary Batalkan Pembelian (Rollback Stok & Kas)
// @Description Membatalkan transaksi belanja, mengembalikan stok gudang ke semula, dan memulihkan saldo kas jika terlanjur lunas.
// @Tags 11. Operasional Inventory
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "ID Pembelian"
// @Success 200 {object} models.MessageResponse "Dibatalkan"
// @Router /api/pembelian/{id} [delete]
func DeletePembelianBahan(c *fiber.Ctx) error {
	id := c.Params("id")
	tx := DB.Begin()

	var p models.NotaPembelian
	// Tarik Nota beserta isinya
	if err := tx.Preload("Details").First(&p, id).Error; err != nil {
		tx.Rollback()
		return c.Status(404).JSON(fiber.Map{"error": "Nota pembelian tidak ditemukan"})
	}

	// 1. Tarik Kembali (Kurangi) Stok semua bahan di dalam nota ini
	for _, d := range p.Details {
		if err := tx.Model(&models.Bahan{}).Where("id = ?", d.BahanID).
			Update("stok", gorm.Expr("stok - ?", d.Qty)).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": "Gagal mengembalikan stok"})
		}
	}

	// 2. Tarik Uang Kembali dari Kas (Hanya jika statusnya sudah Lunas)
	var settingKas models.PengaturanSistem
	tx.Where("key = ?", "ENABLE_KAS_SYNC").First(&settingKas)
	if settingKas.Value == "true" && p.IsLunas {
		noNotaRefBeli := fmt.Sprintf("BELI-%d", p.ID)
		var kasList []models.TransaksiKas
		tx.Where("no_nota_ref = ?", noNotaRefBeli).Find(&kasList)
		for _, k := range kasList {
			TambahSaldoKas(tx, k.Nominal) // Kembalikan uang
		}
		tx.Unscoped().Where("no_nota_ref = ?", noNotaRefBeli).Delete(&models.TransaksiKas{})
	}

	// 3. Hapus Sementara (Soft Delete) Detail (Anak), baru Hapus Induknya
	tx.Where("nota_pembelian_id = ?", p.ID).Delete(&models.NotaPembelianDetail{})
	tx.Delete(&p)

	tx.Commit()
	return c.JSON(fiber.Map{"message": "Nota dibatalkan, semua stok dikurangi, dan kas ditarik kembali!"})
}

// RestorePembelianBahan mengembalikan nota yang di soft-delete
func RestorePembelianBahan(c *fiber.Ctx) error {
	id := c.Params("id")
	tx := DB.Begin()

	var p models.NotaPembelian
	// Cari di tong sampah
	if err := tx.Unscoped().Preload("Details").First(&p, id).Error; err != nil {
		tx.Rollback()
		return c.Status(404).JSON(fiber.Map{"error": "Nota tidak ditemukan di tong sampah"})
	}

	// 1. Hilangkan status "terhapus" (Kembalikan ke alam nyata)
	tx.Unscoped().Model(&p).Update("deleted_at", nil)
	tx.Unscoped().Model(&models.NotaPembelianDetail{}).Where("nota_pembelian_id = ?", p.ID).Update("deleted_at", nil)

	// 2. Kembalikan / Tambah Stok Gudang
	for _, d := range p.Details {
		tx.Model(&models.Bahan{}).Where("id = ?", d.BahanID).Update("stok", gorm.Expr("stok + ?", d.Qty))
	}

	// 3. Potong Kembali Kas (Jika statusnya Lunas)
	var settingKas models.PengaturanSistem
	tx.Where("key = ?", "ENABLE_KAS_SYNC").First(&settingKas)
	if settingKas.Value == "true" && p.IsLunas {
		adminID := c.Locals("admin_id").(uint)
		ketKas := fmt.Sprintf("Pemulihan Belanja Bahan Baku (Nota #%d) - %s", p.ID, p.Keterangan)

		tx.Create(&models.TransaksiKas{
			Tanggal:    p.Tanggal,
			Kategori:   "BAHAN",
			Jenis:      "KELUAR",
			Nominal:    p.TotalBiaya,
			Keterangan: ketKas,
			NoNotaRef:  fmt.Sprintf("BELI-%d", p.ID),
			CreatedBy:  adminID,
		})
		KurangiSaldoKas(tx, p.TotalBiaya)
	}

	tx.Commit()
	return c.JSON(fiber.Map{"message": "Nota berhasil dipulihkan, stok dan kas terpotong kembali!"})
}

// PRODUKSI MASAK
//
// GetProduksiMasak godoc
// @Summary Ambil Catatan Masak Dapur Harian
// @Description Menampilkan riwayat pengadukan resep (batch) berdasarkan hari.
// @Tags 12. Produksi & Dapur
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param tanggal query string false "Tanggal filter (YYYY-MM-DD)"
// @Success 200 {array} models.ProduksiMasak "Berhasil ditarik"
// @Router /api/produksi/masak [get]
func GetProduksiMasak(c *fiber.Ctx) error {
	tanggal := c.Query("tanggal")
	if tanggal == "" {
		tanggal = wib().Format("2006-01-02")
	}
	var masak []models.ProduksiMasak
	DB.Preload("Resep", func(db *gorm.DB) *gorm.DB {
		return db.Unscoped()
	}).Where("tanggal = ?", tanggal).Order("id desc").Find(&masak)
	return c.JSON(masak)
}

// CreateProduksiMasak godoc
// @Summary Catat Masak Adonan Baru
// @Description Menyimpan data adonan dan otomatis memotong seluruh stok fisik bahan baku sesuai rasio resep.
// @Tags 12. Produksi & Dapur
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param payload body models.ProduksiMasakInput true "Data batch masak"
// @Success 200 {object} models.MessageResponse "Dicatat"
// @Router /api/produksi/masak [post]
func CreateProduksiMasak(c *fiber.Ctx) error {
	var input struct {
		Tanggal     string  `json:"tanggal"`
		ResepID     uint    `json:"resep_id"`
		JumlahBatch float64 `json:"jumlah_batch"`
	}
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format data salah"})
	}

	if input.JumlahBatch <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Jumlah batch tidak valid. Harus lebih besar dari 0."})
	}

	tgl, _ := time.Parse("2006-01-02", input.Tanggal)

	// Gunakan Transaction agar kalau gagal potong stok, data masak dibatalkan
	tx := DB.Begin()

	var resep models.Resep
	if err := tx.Preload("BahanDetail").First(&resep, input.ResepID).Error; err != nil {
		tx.Rollback()
		return c.Status(404).JSON(fiber.Map{"error": "Resep tidak ditemukan"})
	}

	totalAdonan := resep.TargetGramasi * input.JumlahBatch

	masak := models.ProduksiMasak{
		Tanggal:     tgl,
		ResepID:     input.ResepID,
		JumlahBatch: input.JumlahBatch,
		TotalAdonan: totalAdonan,
	}

	if err := tx.Create(&masak).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "Gagal mencatat data masak"})
	}

	// OTOMASI: POTONG STOK BAHAN FISIK
	for _, rb := range resep.BahanDetail {
		pengurangan := rb.Kebutuhan * input.JumlahBatch
		if err := tx.Model(&models.Bahan{}).Where("id = ?", rb.BahanID).Update("stok", gorm.Expr("stok - ?", pengurangan)).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": "Gagal memotong stok bahan: " + err.Error()})
		}
	}

	tx.Commit()
	return c.JSON(fiber.Map{"message": "Produksi berhasil dicatat! Stok gudang otomatis terpotong."})
}

// DeleteProduksiMasak godoc
// @Summary Batal Masak Adonan (Rollback Stok Mentah)
// @Description Membatalkan catatan pengadukan resep dan mengembalikan stok fisik bahan baku ke gudang.
// @Tags 12. Produksi & Dapur
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "ID Masak"
// @Success 200 {object} models.MessageResponse "Dibatalkan"
// @Router /api/produksi/masak/{id} [delete]
func DeleteProduksiMasak(c *fiber.Ctx) error {
	id := c.Params("id")
	tx := DB.Begin()

	// 1. Cari data masak yang mau dihapus
	var masak models.ProduksiMasak
	if err := tx.First(&masak, id).Error; err != nil {
		tx.Rollback()
		return c.Status(404).JSON(fiber.Map{"error": "Data masak tidak ditemukan"})
	}

	// 2. Tarik resep beserta detail bahannya
	var resep models.Resep
	if err := tx.Preload("BahanDetail").First(&resep, masak.ResepID).Error; err == nil {
		// 3. Kembalikan (Refund) stok fisik ke gudang
		for _, rb := range resep.BahanDetail {
			pengembalian := rb.Kebutuhan * masak.JumlahBatch
			if err := tx.Model(&models.Bahan{}).Where("id = ?", rb.BahanID).Update("stok", gorm.Expr("stok + ?", pengembalian)).Error; err != nil {
				tx.Rollback()
				return c.Status(500).JSON(fiber.Map{"error": "Gagal mengembalikan stok bahan"})
			}
		}
	}

	// 4. Hapus data masaknya (Gunakan Unscoped agar benar-benar hilang dari database)
	tx.Unscoped().Delete(&masak)
	tx.Commit()

	return c.JSON(fiber.Map{"message": "Data masak dibatalkan, stok bahan mentah dikembalikan!"})
}

// PRODUKSI MATANG
//
// GetProduksiMatang godoc
// @Summary Ambil Hasil Oven Matang Harian
// @Description Menampilkan riwayat hasil matang roti yang sudah masuk ke buffer pengiriman.
// @Tags 12. Produksi & Dapur
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param tanggal query string false "Tanggal filter (YYYY-MM-DD)"
// @Success 200 {array} models.ProduksiMatang "Berhasil ditarik"
// @Router /api/produksi/matang [get]
func GetProduksiMatang(c *fiber.Ctx) error {
	tanggal := c.Query("tanggal")
	if tanggal == "" {
		tanggal = wib().Format("2006-01-02")
	}
	var matang []models.ProduksiMatang
	DB.Preload("Barang", func(db *gorm.DB) *gorm.DB {
		return db.Unscoped()
	}).Where("tanggal = ?", tanggal).Order("id desc").Find(&matang)
	return c.JSON(matang)
}

// CreateProduksiMatang godoc
// @Summary Catat Roti Matang Keluar Oven
// @Description Mencatat perolehan roti fisik dan otomatis memotong stok kardus/plastik kemasan dari gudang.
// @Tags 12. Produksi & Dapur
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param payload body models.ProduksiMatangInput true "Data hasil oven"
// @Success 200 {object} models.MessageResponse "Dicatat"
// @Router /api/produksi/matang [post]
func CreateProduksiMatang(c *fiber.Ctx) error {
	var input struct {
		Tanggal   string `json:"tanggal"`
		BarangID  uint   `json:"barang_id"`
		QtyMatang float64    `json:"qty_matang"`
	}
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format data salah"})
	}

	if input.QtyMatang <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Kuantitas matang tidak valid. Harus lebih besar dari 0."})
	}

	tgl, _ := time.Parse("2006-01-02", input.Tanggal)

	tx := DB.Begin()

	var existing models.ProduksiMatang
	err := tx.Where("tanggal = ? AND barang_id = ?", tgl, input.BarangID).First(&existing).Error

	if err == nil {
		tx.Model(&existing).Update("qty_matang", existing.QtyMatang+input.QtyMatang)
	} else {
		matang := models.ProduksiMatang{Tanggal: tgl, BarangID: input.BarangID, QtyMatang: input.QtyMatang}
		tx.Create(&matang)
	}

	// === BARU: POTONG STOK KEMASAN & KOMPOSIT ===
	var barang models.Barang
	// WAJIB Preload Kemasan dan Komposit beserta detail rasionya
	if err := tx.Preload("Kemasan").Preload("Komposit.ResepKomposit.Details").First(&barang, input.BarangID).Error; err == nil {

		// 1. Potong Kemasan (Logika Lama)
		for _, k := range barang.Kemasan {
			pengurangan := k.Kebutuhan * input.QtyMatang
			tx.Model(&models.Bahan{}).Where("id = ?", k.BahanID).Update("stok", gorm.Expr("stok - ?", pengurangan))
		}

		// 2. Potong Resep Komposit (Logika Potong Pecahan Rasio)
		for _, komp := range barang.Komposit {
			totalKebutuhanKomposit := komp.Kebutuhan * input.QtyMatang // Misal: 40gr * 100pcs = 4000gr

			// Hitung total rasio pembagi (misal 4 + 2 + 7 = 13)
			var totalRasio float64
			for _, detail := range komp.ResepKomposit.Details {
				totalRasio += detail.Rasio
			}

			// Eksekusi potong per bahan dasar butter/coklat
			if totalRasio > 0 {
				for _, detail := range komp.ResepKomposit.Details {
					// RUMUS: (Rasio Bahan / Total Rasio) * Total Kebutuhan Gramasi Matang
					gramasiPotong := (detail.Rasio / totalRasio) * totalKebutuhanKomposit
					tx.Model(&models.Bahan{}).Where("id = ?", detail.BahanID).Update("stok", gorm.Expr("stok - ?", gramasiPotong))
				}
			}
		}
	}

	tx.Commit()
	return c.JSON(fiber.Map{"message": "Hasil matang dicatat & kemasan terpotong!"})
}

// DeleteProduksiMatang godoc
// @Summary Batal Catat Matang (Rollback Stok Kemasan)
// @Description Membatalkan catatan matang dan mengembalikan bahan plastik/kardus ke gudang.
// @Tags 12. Produksi & Dapur
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "ID Matang"
// @Success 200 {object} models.MessageResponse "Dibatalkan"
// @Router /api/produksi/matang/{id} [delete]
func DeleteProduksiMatang(c *fiber.Ctx) error {
	id := c.Params("id")
	tx := DB.Begin()

	// 1. Cari data matang yang mau dihapus
	var matang models.ProduksiMatang
	if err := tx.First(&matang, id).Error; err != nil {
		tx.Rollback()
		return c.Status(404).JSON(fiber.Map{"error": "Data matang tidak ditemukan"})
	}

	// 2. Tarik data barang beserta detail kemasan & kompositnya
	var barang models.Barang
	if err := tx.Preload("Kemasan").Preload("Komposit.ResepKomposit.Details").First(&barang, matang.BarangID).Error; err == nil {

		// 3. Kembalikan (Refund) stok kemasan
		for _, k := range barang.Kemasan {
			pengembalian := k.Kebutuhan * matang.QtyMatang
			tx.Model(&models.Bahan{}).Where("id = ?", k.BahanID).Update("stok", gorm.Expr("stok + ?", pengembalian))
		}

		// 4. Kembalikan (Refund) stok Resep Komposit
		for _, komp := range barang.Komposit {
			totalKebutuhanKomposit := komp.Kebutuhan * matang.QtyMatang

			var totalRasio float64
			for _, detail := range komp.ResepKomposit.Details {
				totalRasio += detail.Rasio
			}

			if totalRasio > 0 {
				for _, detail := range komp.ResepKomposit.Details {
					gramasiKembali := (detail.Rasio / totalRasio) * totalKebutuhanKomposit
					tx.Model(&models.Bahan{}).Where("id = ?", detail.BahanID).Update("stok", gorm.Expr("stok + ?", gramasiKembali))
				}
			}
		}
	}

	// 4. Hapus data matangnya
	tx.Unscoped().Delete(&matang)
	tx.Commit()

	return c.JSON(fiber.Map{"message": "Data matang dibatalkan, stok kemasan dikembalikan!"})
}

// BARANG RUSAK / AFKIR / GRATIS
//
// GetBarangRusak godoc
// @Summary Ambil Riwayat Afkir / Gratisan
// @Description Menampilkan data barang terbuang/afkir harian.
// @Tags 11. Operasional Inventory
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param tanggal query string false "Tanggal filter (YYYY-MM-DD)"
// @Success 200 {array} models.BarangRusak "Berhasil ditarik"
// @Router /api/inventory/rusak [get]
func GetBarangRusak(c *fiber.Ctx) error {
	tanggal := c.Query("tanggal")
	if tanggal == "" {
		tanggal = wib().Format("2006-01-02")
	}
	var rusak []models.BarangRusak
	DB.Preload("Barang", func(db *gorm.DB) *gorm.DB {
		return db.Unscoped()
	}).Where("tanggal = ?", tanggal).Order("id desc").Find(&rusak)
	return c.JSON(rusak)
}

// CreateBarangRusak godoc
// @Summary Catat Barang Rusak / Afkir
// @Description Menyimpan jumlah roti yang dibuang/gratis, akan otomatis mengurangi jumlah Sisa Layak Jual saat Tutup Buku nanti malam.
// @Tags 11. Operasional Inventory
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param payload body models.BarangRusakInput true "Data afkir"
// @Success 200 {object} models.MessageResponse "Dicatat"
// @Router /api/inventory/rusak [post]
func CreateBarangRusak(c *fiber.Ctx) error {
	var input struct {
		Tanggal    string `json:"tanggal"`
		BarangID   uint   `json:"barang_id"`
		Qty        float64    `json:"qty"`
		Keterangan string `json:"keterangan"`
	}
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format salah"})
	}

	if input.Qty <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Kuantitas tidak valid. Harus lebih besar dari 0."})
	}

	tgl, _ := time.Parse("2006-01-02", input.Tanggal)

	rusak := models.BarangRusak{
		Tanggal:    tgl,
		BarangID:   input.BarangID,
		Qty:        input.Qty,
		Keterangan: input.Keterangan,
	}

	if err := DB.Create(&rusak).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Barang afkir/gratis berhasil dicatat!"})
}

// DeleteBarangRusak godoc
// @Summary Batal Catat Afkir
// @Description Menghapus riwayat afkir. Kalkulasi mesin Tutup Buku akan menyesuaikan sendiri.
// @Tags 11. Operasional Inventory
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "ID Afkir"
// @Success 200 {object} models.MessageResponse "Dihapus"
// @Router /api/inventory/rusak/{id} [delete]
func DeleteBarangRusak(c *fiber.Ctx) error {
	id := c.Params("id")

	// Cukup hapus datanya. Mesin Tutup Buku akan otomatis menyesuaikan diri malam harinya!
	if err := DB.Unscoped().Delete(&models.BarangRusak{}, id).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal menghapus data afkir: " + err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Data afkir berhasil dibatalkan!"})
}

// TUTUP BUKU & LAPORAN
//
// 1. FUNGSI TUTUP BUKU BULLETPROOF (Anti Zona Waktu, Mapping Error & Plural Table)
//
// TutupBukuHarian godoc
// @Summary Jalankan Mesin Tutup Buku Dapur (Sinkronisasi Akhir)
// @Description Mesin raksasa yang mengakumulasi hasil matang, dikurangi nota terkirim, dikurangi nota PO, dikurangi afkir, dan akhirnya menyimpulkan Sisa Layak Jual (stok nyantol) serta mencetak Jurnal Efisiensi.
// @Tags 13. Laporan Penutup (End of Day)
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param payload body models.TutupBukuInput true "Tanggal tutup buku"
// @Success 200 {object} models.MessageResponse "Berhasil dikalkulasi"
// @Router /api/produksi/tutup-buku [post]
func TutupBukuHarian(c *fiber.Ctx) error {
	var input struct {
		Tanggal string `json:"tanggal"`
	}
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format salah"})
	}
	// Pakai time.Time agar formatnya sama persis dengan yang disimpan CreateNota
	tgl, _ := time.Parse("2006-01-02", input.Tanggal)

	// -------------------------------------------------------------
	// 1. MENGHITUNG SISA LAYAK JUAL (Kenyataan Matang vs Terkirim)
	// -------------------------------------------------------------
	var matangList []models.ProduksiMatang
	DB.Where("tanggal = ?", tgl).Find(&matangList)

	matangMap := make(map[uint]float64)
	barangMap := make(map[uint]bool)

	var existingSisa []models.SisaLayakJual
	DB.Where("tanggal = ?", tgl).Find(&existingSisa)
	for _, s := range existingSisa {
		barangMap[s.BarangID] = true
	}

	for _, m := range matangList {
		matangMap[m.BarangID] += m.QtyMatang
		barangMap[m.BarangID] = true
	}

	kirimMap := make(map[uint]float64)

	type KirimResult struct {
		BarangID uint
		Total    float64
	}

	// Tarik Nota Reguler
	var kirimReg []KirimResult
	DB.Table("nota_details").
		Select("nota_details.barang_id, COALESCE(SUM(nota_details.banyak_kirim), 0) as total").
		Joins("JOIN nota ON nota.id = nota_details.nota_id").
		Where("nota.tanggal_kirim = ? AND nota.status != 'DIBATALKAN'", tgl). // <--- TAMBAHAN FILTER
		Group("nota_details.barang_id").
		Scan(&kirimReg)

	for _, kr := range kirimReg {
		kirimMap[kr.BarangID] += kr.Total
		barangMap[kr.BarangID] = true
	}

	// Tarik Nota PO (Pesanan)
	var kirimPO []KirimResult
	DB.Table("nota_pesanan_details").
		Select("nota_pesanan_details.barang_id, COALESCE(SUM(nota_pesanan_details.banyak), 0) as total").
		Joins("JOIN nota_pesanans ON nota_pesanans.id = nota_pesanan_details.nota_pesanan_id").
		Where("nota_pesanans.tanggal_kirim = ? AND nota_pesanan_details.barang_id IS NOT NULL AND nota_pesanans.status != 'DIBATALKAN'", tgl). // <--- TAMBAHAN FILTER
		Group("nota_pesanan_details.barang_id").
		Scan(&kirimPO)

	for _, kp := range kirimPO {
		kirimMap[kp.BarangID] += kp.Total
		barangMap[kp.BarangID] = true
	}

	// -------------------------------------------------------------
	// BARU: 1.5 MENGHITUNG BARANG RUSAK / GRATIS (PENGURANG MUTLAK)
	// -------------------------------------------------------------
	var rusakList []models.BarangRusak
	DB.Where("tanggal = ?", tgl).Find(&rusakList)

	rusakMap := make(map[uint]float64)
	for _, r := range rusakList {
		rusakMap[r.BarangID] += r.Qty
		barangMap[r.BarangID] = true
	}

	// Eksekusi Pemotongan Sisa
	for barangID := range barangMap {
		sisa := matangMap[barangID] - kirimMap[barangID] - rusakMap[barangID]

		var slj models.SisaLayakJual
		err := DB.Where("tanggal = ? AND barang_id = ?", tgl, barangID).First(&slj).Error
		if err == nil {
			DB.Model(&slj).Updates(map[string]interface{}{"qty_sisa": sisa})
		} else {
			DB.Create(&models.SisaLayakJual{Tanggal: tgl, BarangID: barangID, QtySisa: sisa})
		}
	}

	// -------------------------------------------------------------
	// 2. MENGHITUNG WASTE DAPUR
	// -------------------------------------------------------------
	var masakList []models.ProduksiMasak
	DB.Where("tanggal = ?", tgl).Find(&masakList)

	resepMap := make(map[uint]bool)
	masakMap := make(map[uint]float64)
	for _, m := range masakList {
		masakMap[m.ResepID] += m.TotalAdonan
	}

	hasilMap := make(map[uint]float64)

	// A. Tambahkan Hasil dari Matang Reguler
	for _, m := range matangList {
		var b models.Barang
		DB.First(&b, m.BarangID)
		if b.ResepID != nil {
			hasilMap[*b.ResepID] += m.QtyMatang * b.KebutuhanAdonan
			resepMap[*b.ResepID] = true
		}
	}

	// B. (BARU!) Tambahkan Hasil dari Pesanan PO Kustom + POTONG KEMASAN & KOMPOSIT
	var poKustom []models.NotaPesananDetail
	DB.Joins("JOIN nota_pesanans ON nota_pesanans.id = nota_pesanan_details.nota_pesanan_id").
		Preload("KemasanDetail").
		Preload("KompositDetail.ResepKomposit.Details"). // <--- WAJIB PRELOAD RELASI KOMPOSIT
		Where("nota_pesanans.tanggal_kirim = ? AND nota_pesanans.status != 'DIBATALKAN'", tgl).
		Find(&poKustom)

	for _, pk := range poKustom {
		if pk.ResepID != nil {
			hasilMap[*pk.ResepID] += (pk.Gramasi * float64(pk.Banyak))
			resepMap[*pk.ResepID] = true
		}

		// LOGIKA POTONG KEMASAN KHUSUS KUSTOM (BarangID == nil)
		// ++ TAMBAHKAN SYARAT: Hanya potong JIKA BELUM TERPOTONG (!pk.IsKemasanTerpotong)
		if pk.BarangID == nil && len(pk.KemasanDetail) > 0 && !pk.IsKemasanTerpotong {
			for _, k := range pk.KemasanDetail {
				totalPotong := float64(pk.Banyak) * k.Kebutuhan
				DB.Model(&models.Bahan{}).Where("id = ?", k.BahanID).
					Update("stok", gorm.Expr("stok - ?", totalPotong))
			}
			// ++ KUNCI GEMBOKNYA: Update baris pesanan ini agar tidak dipotong lagi besok/nanti
			DB.Model(&models.NotaPesananDetail{}).Where("id = ?", pk.ID).Update("is_kemasan_terpotong", true)
		}

		// LOGIKA POTONG KOMPOSIT KHUSUS KUSTOM
		if pk.BarangID == nil && len(pk.KompositDetail) > 0 && !pk.IsKompositTerpotong {
			for _, k := range pk.KompositDetail {
				totalKebutuhanKomposit := k.Kebutuhan * float64(pk.Banyak)

				var totalRasio float64
				for _, detail := range k.ResepKomposit.Details {
					totalRasio += detail.Rasio
				}

				if totalRasio > 0 {
					for _, detail := range k.ResepKomposit.Details {
						gramasiPotong := (detail.Rasio / totalRasio) * totalKebutuhanKomposit
						DB.Model(&models.Bahan{}).Where("id = ?", detail.BahanID).
							Update("stok", gorm.Expr("stok - ?", gramasiPotong))
					}
				}
			}
			DB.Model(&models.NotaPesananDetail{}).Where("id = ?", pk.ID).Update("is_komposit_terpotong", true)
		}
	}

	var existingJurnal []models.JurnalEfisiensi
	DB.Where("tanggal = ?", tgl).Find(&existingJurnal)
	for _, j := range existingJurnal {
		resepMap[j.ResepID] = true
	}

	// Eksekusi Pembuatan Jurnal Efisiensi
	for resepID := range resepMap {
		modal := masakMap[resepID]
		hasil := hasilMap[resepID]
		waste := modal - hasil
		kinerja := 0.0
		if modal > 0 {
			kinerja = (hasil / modal) * 100
		}

		var jr models.JurnalEfisiensi
		err := DB.Where("tanggal = ? AND resep_id = ?", tgl, resepID).First(&jr).Error
		if err == nil {
			DB.Model(&jr).Updates(map[string]interface{}{"modal_adonan": modal, "hasil_roti": hasil, "selisih_waste": waste, "kinerja": kinerja})
		} else {
			DB.Create(&models.JurnalEfisiensi{Tanggal: tgl, ResepID: resepID, ModalAdonan: modal, HasilRoti: hasil, SelisihWaste: waste, Kinerja: kinerja})
		}
	}

	return c.JSON(fiber.Map{"message": "Tutup buku berhasil dikalkulasi!"})
}

// 2. FUNGSI TAMPIL LAYAR LAPORAN (Menampilkan Angka 0 Pcs)
//
// GetJurnalTutupBuku godoc
// @Summary Ambil Hasil Jurnal Tutup Buku
// @Description Mengambil laporan Jurnal Efisiensi (Waste) dan tabel sisa layak jual yang dihasilkan mesin tutup buku.
// @Tags 13. Laporan Penutup (End of Day)
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param tanggal query string true "Tanggal tutup buku (YYYY-MM-DD)"
// @Success 200 {object} models.JurnalTutupBukuResponse "Data jurnal berhasil ditarik"
// @Router /api/produksi/jurnal [get]
func GetJurnalTutupBuku(c *fiber.Ctx) error {
	tgl := c.Query("tanggal")
	var jurnal []models.JurnalEfisiensi

	var rawSisa []struct {
		BarangID  uint
		TotalSisa float64
	}

	// AKUMULASI SEMUA SISA (Filter HAVING != 0 dihapus agar angka 0 tetap tampil sebagai bukti terjual habis)
	// UPDATE: Menghapus batas masa_simpan agar perhitungan SUM() selalu akurat dan nilai negatif penjualan punya pasangan produksinya.
	DB.Raw(`
		SELECT sisa_layak_juals.barang_id, COALESCE(SUM(sisa_layak_juals.qty_sisa), 0) as total_sisa 
		FROM sisa_layak_juals 
		WHERE DATE(sisa_layak_juals.tanggal) <= CAST(? AS DATE)
		GROUP BY sisa_layak_juals.barang_id
	`, tgl).Scan(&rawSisa)

	var sisaAkhir []models.SisaLayakJual
	for _, rs := range rawSisa {
		var b models.Barang
		DB.Unscoped().First(&b, rs.BarangID)
		sisaAkhir = append(sisaAkhir, models.SisaLayakJual{
			BarangID: b.ID,
			Barang:   b,
			QtySisa:  rs.TotalSisa,
		})
	}

	DB.Preload("Resep", func(db *gorm.DB) *gorm.DB {
		return db.Unscoped()
	}).Where("tanggal = ?", tgl).Find(&jurnal)
	return c.JSON(fiber.Map{"jurnal": jurnal, "sisa": sisaAkhir})
}

// KONVERSI (TARIK SISA KEMARIN)
//
// GetSisaLayakJualKemarin godoc
// @Summary Tarik Sisa Layak Jual (H-1)
// @Description Mengambil akumulasi sisa layak jual dari awal operasi hingga H-1.
// @Tags 11. Operasional Inventory
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param tanggal query string true "Tanggal pengiriman hari ini (YYYY-MM-DD)"
// @Success 200 {array} models.SisaLayakJual "Data sisa kemarin ditarik"
// @Router /api/konversi/sisa-kemarin [get]
func GetSisaLayakJualKemarin(c *fiber.Ctx) error {
	tgl := c.Query("tanggal")

	var sisaAktif []models.SisaLayakJual

	var rawSisa []struct {
		BarangID  uint
		TotalSisa float64
	}

	// UPDATE: Menghitung akumulasi SUM() secara backend agar tidak ada utang siluman yang dibawa per baris per hari.
	// Mengabaikan masa_simpan karena kedaluwarsa sudah diproses melalui fitur BarangRusak/Afkir secara mutlak.
	err := DB.Raw(`
		SELECT sisa_layak_juals.barang_id, COALESCE(SUM(sisa_layak_juals.qty_sisa), 0) as total_sisa 
		FROM sisa_layak_juals 
		WHERE DATE(sisa_layak_juals.tanggal) < CAST(? AS DATE)
		GROUP BY sisa_layak_juals.barang_id
	`, tgl).Scan(&rawSisa).Error

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	for _, rs := range rawSisa {
		// Kita kembalikan nilai 0 juga ke frontend untuk menandakan bahwa item tsb pernah ada tapi sudah habis
		var b models.Barang
		DB.Unscoped().First(&b, rs.BarangID)
		
		sisaAktif = append(sisaAktif, models.SisaLayakJual{
			BarangID: b.ID,
			Barang:   b,
			QtySisa:  rs.TotalSisa,
		})
	}

	return c.JSON(sisaAktif)
}

// STOCK OPNAME (SIDAK GUDANG)
//
// GetOpname godoc
// @Summary Ambil Riwayat Stock Opname (Sidak Fisik)
// @Description Menampilkan histori koreksi paksa jumlah stok bahan fisik di gudang.
// @Tags 11. Operasional Inventory
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {array} models.StockOpname "Berhasil ditarik"
// @Router /api/opname [get]
func GetOpname(c *fiber.Ctx) error {
	var opname []models.StockOpname
	query := DB.Preload("Bahan").Order("id desc")

	startDate := c.Query("start")
	endDate := c.Query("end")

	if startDate != "" && endDate != "" {
		query = query.Where("tanggal >= ? AND tanggal <= ?", startDate, endDate)
	}

	query.Find(&opname)
	return c.JSON(opname)
}

// CreateOpname godoc
// @Summary Eksekusi Stock Opname
// @Description Memperbarui stok mutlak database dengan stok fisik nyata (menyesuaikan selisih/tumpah secara paksa).
// @Tags 11. Operasional Inventory
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param payload body models.StockOpnameInput true "Data penyesuaian stok fisik"
// @Success 200 {object} models.MessageResponse "Opname dicatat"
// @Router /api/opname [post]
func CreateOpname(c *fiber.Ctx) error {
	var input struct {
		BahanID    uint    `json:"bahan_id"`
		StokFisik  float64 `json:"stok_fisik"`
		Keterangan string  `json:"keterangan"`
	}
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format data salah"})
	}

	if input.StokFisik < 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Stok fisik tidak valid. Tidak boleh minus."})
	}

	tx := DB.Begin()
	var bahan models.Bahan
	if err := tx.First(&bahan, input.BahanID).Error; err != nil {
		tx.Rollback()
		return c.Status(404).JSON(fiber.Map{"error": "Bahan tidak ditemukan"})
	}

	selisih := input.StokFisik - bahan.Stok

	opname := models.StockOpname{
		Tanggal:    wib(),
		BahanID:    input.BahanID,
		StokSistem: bahan.Stok,
		StokFisik:  input.StokFisik,
		Selisih:    selisih,
		Keterangan: input.Keterangan,
	}
	if err := tx.Create(&opname).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// Update stok master bahan sesuai fisik nyata
	tx.Model(&bahan).Update("stok", input.StokFisik)
	tx.Commit()

	return c.JSON(fiber.Map{"message": "Stock Opname berhasil dicatat!"})
}

// GetKonversiBahan godoc
// @Summary Riwayat Konversi Barang (Pecah Barang)
// @Description Menampilkan riwayat pemotongan barang
// @Tags 11. Operasional Inventory
// @Accept json
// @Produce json
// @Security BearerAuth
// @Router /api/inventory/pecah-barang [get]
func GetKonversiBahan(c *fiber.Ctx) error {
	var riwayat []models.KonversiBahan
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	query := DB.Preload("BahanAsal").Preload("Details.BahanHasil").Order("tanggal desc")

	if startDate != "" && endDate != "" {
		query = query.Where("DATE(tanggal) >= ? AND DATE(tanggal) <= ?", startDate, endDate)
	}

	query.Find(&riwayat)
	return c.JSON(riwayat)
}

// CreateKonversiBahan godoc
// @Summary Pecah Barang
// @Description Mengonversi 1 barang utuh menjadi banyak pecahan potongan
// @Tags 11. Operasional Inventory
// @Accept json
// @Produce json
// @Security BearerAuth
// @Router /api/inventory/pecah-barang [post]
func CreateKonversiBahan(c *fiber.Ctx) error {
	var input struct {
		Tanggal     string `json:"tanggal"`
		BahanAsalID uint   `json:"bahan_asal_id"`
		QtyAsal     float64 `json:"qty_asal"`
		Keterangan  string `json:"keterangan"`
		Details     []struct {
			BahanHasilID uint    `json:"bahan_hasil_id"`
			QtyHasil     float64 `json:"qty_hasil"`
		} `json:"details"`
	}

	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	if input.QtyAsal <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Kuantitas asal tidak valid. Harus lebih besar dari 0."})
	}
	for _, d := range input.Details {
		if d.QtyHasil <= 0 {
			return c.Status(400).JSON(fiber.Map{"error": "Kuantitas hasil tidak valid. Harus lebih besar dari 0."})
		}
	}

	tanggal, _ := time.Parse("2006-01-02", input.Tanggal)
	if input.Tanggal == "" {
		tanggal = wib()
	}

	tx := DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 1. Kurangi stok bahan asal
	var bahanAsal models.Bahan
	if err := tx.First(&bahanAsal, input.BahanAsalID).Error; err != nil {
		tx.Rollback()
		return c.Status(404).JSON(fiber.Map{"error": "Bahan asal tidak ditemukan"})
	}
	if bahanAsal.Stok < input.QtyAsal {
		tx.Rollback()
		return c.Status(400).JSON(fiber.Map{"error": "Stok bahan asal tidak cukup"})
	}
	tx.Model(&bahanAsal).UpdateColumn("stok", gorm.Expr("stok - ?", input.QtyAsal))

	// 2. Buat Record Konversi
	konversi := models.KonversiBahan{
		Tanggal:     tanggal,
		BahanAsalID: input.BahanAsalID,
		QtyAsal:     input.QtyAsal,
		Keterangan:  input.Keterangan,
	}
	if err := tx.Create(&konversi).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 3. Tambah stok bahan hasil
	for _, d := range input.Details {
		detail := models.KonversiBahanDetail{
			KonversiBahanID: konversi.ID,
			BahanHasilID:    d.BahanHasilID,
			QtyHasil:        d.QtyHasil,
		}
		if err := tx.Create(&detail).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		
		// Update stok bahan hasil
		tx.Model(&models.Bahan{}).Where("id = ?", d.BahanHasilID).UpdateColumn("stok", gorm.Expr("stok + ?", d.QtyHasil))
	}

	tx.Commit()
	return c.JSON(fiber.Map{"message": "Barang berhasil dipecah!"})
}

// DeleteKonversiBahan godoc
// @Summary Batal Pecah Barang
// @Description Membatalkan riwayat konversi dan mengembalikan stok
// @Tags 11. Operasional Inventory
// @Accept json
// @Produce json
// @Security BearerAuth
// @Router /api/inventory/pecah-barang/{id} [delete]
func DeleteKonversiBahan(c *fiber.Ctx) error {
	id := c.Params("id")
	var konversi models.KonversiBahan
	
	if err := DB.Preload("Details").First(&konversi, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Riwayat konversi tidak ditemukan"})
	}

	tx := DB.Begin()

	// 1. Kembalikan stok bahan asal (ditambah)
	tx.Model(&models.Bahan{}).Where("id = ?", konversi.BahanAsalID).UpdateColumn("stok", gorm.Expr("stok + ?", konversi.QtyAsal))

	// 2. Tarik kembali stok bahan hasil (dikurangi)
	for _, d := range konversi.Details {
		tx.Model(&models.Bahan{}).Where("id = ?", d.BahanHasilID).UpdateColumn("stok", gorm.Expr("stok - ?", d.QtyHasil))
	}

	// 3. Hapus history
	tx.Delete(&models.KonversiBahanDetail{}, "konversi_bahan_id = ?", konversi.ID)
	tx.Delete(&konversi)

	tx.Commit()
	return c.JSON(fiber.Map{"message": "Riwayat konversi dibatalkan, stok telah dikembalikan."})
}
