package main

import (
	"backend/models"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

func GetPembelianBahan(c *fiber.Ctx) error {
	start := c.Query("start")
	end := c.Query("end")

	var beli []models.PembelianBahan

	query := DB.Preload("Bahan", func(db *gorm.DB) *gorm.DB {
		return db.Unscoped()
	})

	// Jika frontend mengirim parameter tanggal, filter query-nya
	if start != "" && end != "" {
		query = query.Where("tanggal >= ? AND tanggal <= ?", start, end)
	}

	// Tarik riwayat belanja beserta nama bahannya, urutkan dari yang terbaru
	if err := query.Order("tanggal desc, id desc").Find(&beli).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(beli)
}

// PEMBELIAN BAHAN (UPDATE OTOMATIS)
func CreatePembelianBahan(c *fiber.Ctx) error {
	var input struct {
		Tanggal         string  `json:"tanggal"`
		BahanID         uint    `json:"bahan_id"`
		Qty             float64 `json:"qty"`
		HargaBeliSatuan float64 `json:"harga_beli_satuan"`
		Keterangan      string  `json:"keterangan"`
		IsLunas         bool    `json:"is_lunas"`
	}

	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format data salah"})
	}

	tgl, _ := time.Parse("2006-01-02", input.Tanggal)
	totalBiaya := input.Qty * input.HargaBeliSatuan
	// adminID := c.Locals("admin_id").(uint)

	// Gunakan Transaction agar jika salah satu gagal, semuanya batal (Aman untuk Akuntansi)
	tx := DB.Begin()

	// Ambil detail bahan untuk mendapatkan nama & satuan guna keterangan di Kas
	// var bahan models.Bahan
	// if err := tx.First(&bahan, input.BahanID).Error; err != nil {
	// 	tx.Rollback()
	// 	return c.Status(404).JSON(fiber.Map{"error": "Bahan tidak ditemukan"})
	// }

	// 1. Simpan Riwayat Belanja
	pembelian := models.PembelianBahan{
		Tanggal:         tgl,
		BahanID:         input.BahanID,
		Qty:             input.Qty,
		HargaBeliSatuan: input.HargaBeliSatuan,
		TotalBiaya:      totalBiaya,
		Keterangan:      input.Keterangan,
		IsLunas:         input.IsLunas,
	}

	if err := tx.Create(&pembelian).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "Gagal mencatat pembelian: " + err.Error()})
	}

	// 2. Tambah Stok Bahan & Timpa Harga Saat Ini
	if err := tx.Model(&models.Bahan{}).Where("id = ?", input.BahanID).Updates(map[string]interface{}{
		"stok":           gorm.Expr("stok + ?", input.Qty),
		"harga_saat_ini": input.HargaBeliSatuan,
	}).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "Gagal update stok bahan: " + err.Error()})
	}

	// ==========================================
	// FULL SYNC KAS: HANYA JIKA LANGSUNG LUNAS
	// ==========================================
	var settingKas models.PengaturanSistem
	DB.Where("key = ?", "ENABLE_KAS_SYNC").First(&settingKas)

	if settingKas.Value == "true" {
		if input.IsLunas {
			adminID := c.Locals("admin_id").(uint)
			var bahan models.Bahan
			tx.First(&bahan, input.BahanID)

			ketKas := fmt.Sprintf("Beli Bahan: %s (%v %s) - %s", bahan.NamaBahan, input.Qty, bahan.Satuan, input.Keterangan)
			if err := tx.Create(&models.TransaksiKas{
				Tanggal:    time.Now(),
				Kategori:   "BAHAN",
				Jenis:      "KELUAR",
				Nominal:    totalBiaya,
				Keterangan: ketKas,
				CreatedBy:  adminID,
			}).Error; err != nil {
				tx.Rollback()
				return c.Status(500).JSON(fiber.Map{"error": "Gagal potong kas"})
			}
		}
	}

	tx.Commit()
	return c.JSON(fiber.Map{"message": "Pembelian berhasil dicatat dan stok ditambahkan!"})
}

// FUNGSI BARU: UBAH STATUS BAYAR (LUNAS <-> HUTANG)
func UpdateStatusPembelian(c *fiber.Ctx) error {
	id := c.Params("id")
	var input struct {
		IsLunas bool `json:"is_lunas"`
	}

	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format data salah"})
	}

	tx := DB.Begin()
	var p models.PembelianBahan
	if err := tx.Preload("Bahan").First(&p, id).Error; err != nil {
		tx.Rollback()
		return c.Status(404).JSON(fiber.Map{"error": "Data pembelian tidak ditemukan"})
	}

	// Update status di database
	if err := tx.Model(&p).Update("is_lunas", input.IsLunas).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "Gagal update status"})
	}

	// ==========================================
	// FULL SYNC KAS: POTONG ATAU KEMBALIKAN KAS
	// ==========================================
	var settingKas models.PengaturanSistem
	DB.Where("key = ?", "ENABLE_KAS_SYNC").First(&settingKas)

	if settingKas.Value == "true" {

		adminID := c.Locals("admin_id").(uint)

		// Kita buat ID referensi unik agar mudah dicari saat mau dihapus
		noNotaRefBeli := fmt.Sprintf("BELI-%d", p.ID)

		if input.IsLunas {
			// JIKA JADI LUNAS -> POTONG KAS (KAS KELUAR)
			ketKas := fmt.Sprintf("Pelunasan Bahan: %s (%v %s) - %s", p.Bahan.NamaBahan, p.Qty, p.Bahan.Satuan, p.Keterangan)

			// Cek dulu apakah sudah ada (biar tidak dobel)
			var existingKas models.TransaksiKas
			if err := tx.Where("no_nota_ref = ?", noNotaRefBeli).First(&existingKas).Error; err != nil {
				tx.Create(&models.TransaksiKas{
					Tanggal:    time.Now(),
					Kategori:   "BAHAN",
					Jenis:      "KELUAR",
					Nominal:    p.TotalBiaya,
					Keterangan: ketKas,
					NoNotaRef:  noNotaRefBeli,
					CreatedBy:  adminID,
				})
			}
		} else {
			// JIKA JADI HUTANG -> TARIK KEMBALI UANGNYA DARI KAS (HAPUS KAS KELUAR TADI)
			tx.Unscoped().Where("no_nota_ref = ?", noNotaRefBeli).Delete(&models.TransaksiKas{})
		}
	}

	tx.Commit()
	return c.JSON(fiber.Map{"message": "Status pembayaran berhasil diupdate!"})
}

// FUNGSI BARU: BATALKAN PEMBELIAN & REFUND STOK/KAS
func DeletePembelianBahan(c *fiber.Ctx) error {
	id := c.Params("id")
	tx := DB.Begin()

	var p models.PembelianBahan
	if err := tx.First(&p, id).Error; err != nil {
		tx.Rollback()
		return c.Status(404).JSON(fiber.Map{"error": "Data pembelian tidak ditemukan"})
	}

	// 1. Tarik Kembali (Kurangi) Stok dari Master Bahan
	if err := tx.Model(&models.Bahan{}).Where("id = ?", p.BahanID).
		Update("stok", gorm.Expr("stok - ?", p.Qty)).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "Gagal mengembalikan stok"})
	}

	// 2. Tarik Uang Kembali dari Kas (Hanya jika statusnya sudah Lunas)
	var settingKas models.PengaturanSistem
	tx.Where("key = ?", "ENABLE_KAS_SYNC").First(&settingKas)

	if settingKas.Value == "true" && p.IsLunas {
		// Gunakan kode referensi yang sama dengan yang kita buat di UpdateStatusPembelian
		noNotaRefBeli := fmt.Sprintf("BELI-%d", p.ID)
		tx.Unscoped().Where("no_nota_ref = ?", noNotaRefBeli).Delete(&models.TransaksiKas{})
	}

	// 3. Hapus Permanen Riwayat Pembelian Ini
	tx.Unscoped().Delete(&p)

	tx.Commit()
	return c.JSON(fiber.Map{"message": "Pembelian dibatalkan, stok dikurangi, dan kas ditarik kembali!"})
}

// PRODUKSI MASAK
func GetProduksiMasak(c *fiber.Ctx) error {
	tanggal := c.Query("tanggal")
	if tanggal == "" {
		tanggal = time.Now().Format("2006-01-02")
	}
	var masak []models.ProduksiMasak
	DB.Preload("Resep", func(db *gorm.DB) *gorm.DB {
		return db.Unscoped()
	}).Where("tanggal = ?", tanggal).Order("id desc").Find(&masak)
	return c.JSON(masak)
}

func CreateProduksiMasak(c *fiber.Ctx) error {
	var input struct {
		Tanggal     string  `json:"tanggal"`
		ResepID     uint    `json:"resep_id"`
		JumlahBatch float64 `json:"jumlah_batch"`
	}
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format data salah"})
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
func GetProduksiMatang(c *fiber.Ctx) error {
	tanggal := c.Query("tanggal")
	if tanggal == "" {
		tanggal = time.Now().Format("2006-01-02")
	}
	var matang []models.ProduksiMatang
	DB.Preload("Barang", func(db *gorm.DB) *gorm.DB {
		return db.Unscoped()
	}).Where("tanggal = ?", tanggal).Order("id desc").Find(&matang)
	return c.JSON(matang)
}

func CreateProduksiMatang(c *fiber.Ctx) error {
	var input struct {
		Tanggal   string `json:"tanggal"`
		BarangID  uint   `json:"barang_id"`
		QtyMatang int    `json:"qty_matang"`
	}
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format data salah"})
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

	// === BARU: POTONG STOK KEMASAN ===
	var barang models.Barang
	if err := tx.Preload("Kemasan").First(&barang, input.BarangID).Error; err == nil {
		for _, k := range barang.Kemasan {
			pengurangan := k.Kebutuhan * float64(input.QtyMatang)
			tx.Model(&models.Bahan{}).Where("id = ?", k.BahanID).Update("stok", gorm.Expr("stok - ?", pengurangan))
		}
	}

	tx.Commit()
	return c.JSON(fiber.Map{"message": "Hasil matang dicatat & kemasan terpotong!"})
}

func DeleteProduksiMatang(c *fiber.Ctx) error {
	id := c.Params("id")
	tx := DB.Begin()

	// 1. Cari data matang yang mau dihapus
	var matang models.ProduksiMatang
	if err := tx.First(&matang, id).Error; err != nil {
		tx.Rollback()
		return c.Status(404).JSON(fiber.Map{"error": "Data matang tidak ditemukan"})
	}

	// 2. Tarik data barang beserta detail kemasannya
	var barang models.Barang
	if err := tx.Preload("Kemasan").First(&barang, matang.BarangID).Error; err == nil {
		// 3. Kembalikan (Refund) stok kemasan ke gudang
		for _, k := range barang.Kemasan {
			pengembalian := k.Kebutuhan * float64(matang.QtyMatang)
			tx.Model(&models.Bahan{}).Where("id = ?", k.BahanID).Update("stok", gorm.Expr("stok + ?", pengembalian))
		}
	}

	// 4. Hapus data matangnya
	tx.Unscoped().Delete(&matang)
	tx.Commit()

	return c.JSON(fiber.Map{"message": "Data matang dibatalkan, stok kemasan dikembalikan!"})
}

// BARANG RUSAK / AFKIR / GRATIS
func GetBarangRusak(c *fiber.Ctx) error {
	tanggal := c.Query("tanggal")
	if tanggal == "" {
		tanggal = time.Now().Format("2006-01-02")
	}
	var rusak []models.BarangRusak
	DB.Preload("Barang", func(db *gorm.DB) *gorm.DB {
		return db.Unscoped()
	}).Where("tanggal = ?", tanggal).Order("id desc").Find(&rusak)
	return c.JSON(rusak)
}

func CreateBarangRusak(c *fiber.Ctx) error {
	var input struct {
		Tanggal    string `json:"tanggal"`
		BarangID   uint   `json:"barang_id"`
		Qty        int    `json:"qty"`
		Keterangan string `json:"keterangan"`
	}
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format salah"})
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

func DeleteBarangRusak(c *fiber.Ctx) error {
	id := c.Params("id")

	// Cukup hapus datanya. Mesin Tutup Buku akan otomatis menyesuaikan diri malam harinya!
	if err := DB.Unscoped().Delete(&models.BarangRusak{}, id).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal menghapus data afkir: " + err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Data afkir berhasil dibatalkan!"})
}

// TUTUP BUKU & LAPORAN

// 1. FUNGSI TUTUP BUKU BULLETPROOF (Anti Zona Waktu, Mapping Error & Plural Table)
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

	matangMap := make(map[uint]int)
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

	kirimMap := make(map[uint]int)

	type KirimResult struct {
		BarangID uint
		Total    int
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

	rusakMap := make(map[uint]int)
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
			hasilMap[*b.ResepID] += float64(m.QtyMatang) * b.KebutuhanAdonan
			resepMap[*b.ResepID] = true
		}
	}

	// B. (BARU!) Tambahkan Hasil dari Pesanan PO Kustom + POTONG KEMASAN
	var poKustom []models.NotaPesananDetail
	DB.Joins("JOIN nota_pesanans ON nota_pesanans.id = nota_pesanan_details.nota_pesanan_id").
		Preload("KemasanDetail"). // <--- WAJIB PRELOAD RELASI BARU
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
func GetJurnalTutupBuku(c *fiber.Ctx) error {
	tgl := c.Query("tanggal")
	var jurnal []models.JurnalEfisiensi

	var rawSisa []struct {
		BarangID  uint
		TotalSisa int
	}

	// AKUMULASI SEMUA SISA (Filter HAVING != 0 dihapus agar angka 0 tetap tampil sebagai bukti terjual habis)
	DB.Raw(`
		SELECT sisa_layak_juals.barang_id, COALESCE(SUM(sisa_layak_juals.qty_sisa), 0) as total_sisa 
		FROM sisa_layak_juals 
		JOIN barangs ON barangs.id = sisa_layak_juals.barang_id
		WHERE DATE(sisa_layak_juals.tanggal) >= (CAST(? AS DATE) - (barangs.masa_simpan * INTERVAL '1 day'))
		AND DATE(sisa_layak_juals.tanggal) <= CAST(? AS DATE)
		GROUP BY sisa_layak_juals.barang_id
	`, tgl, tgl).Scan(&rawSisa)

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
func GetSisaLayakJualKemarin(c *fiber.Ctx) error {
	tgl := c.Query("tanggal")

	var sisaAktif []models.SisaLayakJual

	// RUMUS PINTAR: Hapus syarat qty_sisa > 0
	// Agar jika hari ini kita kirim lebih banyak dari yang dimasak (ambil stok kemarin),
	// angka pengurangnya (minus) bisa ikut menjumlahkan dan menyeimbangkan stok besok!
	err := DB.Joins("JOIN barangs ON barangs.id = sisa_layak_juals.barang_id").
		Where("DATE(sisa_layak_juals.tanggal) >= (CAST(? AS DATE) - (barangs.masa_simpan * INTERVAL '1 day'))", tgl).
		Where("DATE(sisa_layak_juals.tanggal) < CAST(? AS DATE)", tgl).
		Find(&sisaAktif).Error

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(sisaAktif)
}

// STOCK OPNAME (SIDAK GUDANG)
func GetOpname(c *fiber.Ctx) error {
	var opname []models.StockOpname
	DB.Preload("Bahan").Order("id desc").Limit(50).Find(&opname)
	return c.JSON(opname)
}

func CreateOpname(c *fiber.Ctx) error {
	var input struct {
		BahanID    uint    `json:"bahan_id"`
		StokFisik  float64 `json:"stok_fisik"`
		Keterangan string  `json:"keterangan"`
	}
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format data salah"})
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
