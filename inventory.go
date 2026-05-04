package main

import (
	"backend/models"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// HELPER WIB
func wib() time.Time {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	return time.Now().In(loc)
}

// HANDLER INVENTORY: MASTER BAHAN & PEMBELIAN
func GetBahan(c *fiber.Ctx) error {
	var bahan []models.Bahan
	if err := DB.Order("nama_bahan asc").Find(&bahan).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(bahan)
}

func CreateBahan(c *fiber.Ctx) error {
	var input models.Bahan
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if err := DB.Create(&input).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(input)
}

func UpdateBahan(c *fiber.Ctx) error {
	id := c.Params("id")
	var input models.Bahan
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	var bahan models.Bahan
	if err := DB.First(&bahan, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Bahan tidak ditemukan"})
	}

	DB.Model(&bahan).Updates(input)
	return c.JSON(fiber.Map{"message": "Bahan berhasil diupdate", "data": bahan})
}

func DeleteBahan(c *fiber.Ctx) error {
	id := c.Params("id")
	if err := DB.Delete(&models.Bahan{}, id).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Bahan berhasil dihapus"})
}

// PEMBELIAN BAHAN (UPDATE OTOMATIS)
func CreatePembelianBahan(c *fiber.Ctx) error {
	var input struct {
		Tanggal         string  `json:"tanggal"`
		BahanID         uint    `json:"bahan_id"`
		Qty             float64 `json:"qty"`
		HargaBeliSatuan float64 `json:"harga_beli_satuan"`
		Keterangan      string  `json:"keterangan"`
	}

	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format data salah"})
	}

	tgl, _ := time.Parse("2006-01-02", input.Tanggal)
	totalBiaya := input.Qty * input.HargaBeliSatuan

	// Gunakan Transaction agar jika salah satu gagal, semuanya batal (Aman untuk Akuntansi)
	tx := DB.Begin()

	// 1. Simpan Riwayat Belanja
	pembelian := models.PembelianBahan{
		Tanggal:         tgl,
		BahanID:         input.BahanID,
		Qty:             input.Qty,
		HargaBeliSatuan: input.HargaBeliSatuan,
		TotalBiaya:      totalBiaya,
		Keterangan:      input.Keterangan,
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

	tx.Commit()
	return c.JSON(fiber.Map{"message": "Pembelian berhasil dicatat dan stok ditambahkan!"})
}

// HANDLER INVENTORY: MASTER RESEP
func GetResep(c *fiber.Ctx) error {
	var resep []models.Resep
	// Preload isi resep beserta nama bahan-bahannya
	if err := DB.Preload("BahanDetail.Bahan").Find(&resep).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(resep)
}

func CreateResep(c *fiber.Ctx) error {
	var input struct {
		NamaResep     string  `json:"nama_resep"`
		TargetGramasi float64 `json:"target_gramasi"`
		BahanDetail   []struct {
			BahanID   uint    `json:"bahan_id"`
			Kebutuhan float64 `json:"kebutuhan"`
		} `json:"bahan_detail"`
	}

	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format data salah"})
	}

	resep := models.Resep{
		NamaResep:     input.NamaResep,
		TargetGramasi: input.TargetGramasi,
	}

	for _, b := range input.BahanDetail {
		resep.BahanDetail = append(resep.BahanDetail, models.ResepBahan{
			BahanID:   b.BahanID,
			Kebutuhan: b.Kebutuhan,
		})
	}

	if err := DB.Create(&resep).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Resep berhasil dibuat!", "id": resep.ID})
}

func UpdateResep(c *fiber.Ctx) error {
	id := c.Params("id")
	var input struct {
		NamaResep     string  `json:"nama_resep"`
		TargetGramasi float64 `json:"target_gramasi"`
		BahanDetail   []struct {
			BahanID   uint    `json:"bahan_id"`
			Kebutuhan float64 `json:"kebutuhan"`
		} `json:"bahan_detail"`
	}

	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format data salah"})
	}

	// Hapus bahan-bahan lama
	DB.Where("resep_id = ?", id).Delete(&models.ResepBahan{})

	// Insert bahan-bahan baru
	var newBahan []models.ResepBahan
	parsedID, _ := strconv.Atoi(id)
	for _, b := range input.BahanDetail {
		newBahan = append(newBahan, models.ResepBahan{
			ResepID:   uint(parsedID),
			BahanID:   b.BahanID,
			Kebutuhan: b.Kebutuhan,
		})
	}
	DB.Create(&newBahan)

	// Update Header Resep
	DB.Model(&models.Resep{}).Where("id = ?", id).Updates(map[string]interface{}{
		"nama_resep":     input.NamaResep,
		"target_gramasi": input.TargetGramasi,
	})

	return c.JSON(fiber.Map{"message": "Resep berhasil diupdate!"})
}

func DeleteResep(c *fiber.Ctx) error {
	id := c.Params("id")
	// Soft delete resep, bahan detail akan terikat oleh relasi tapi resepnya hilang
	if err := DB.Delete(&models.Resep{}, id).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Resep berhasil dihapus"})
}

// HANDLER INVENTORY: PRODUKSI HARIAN

func GetProduksiMasak(c *fiber.Ctx) error {
	tanggal := c.Query("tanggal")
	if tanggal == "" {
		tanggal = time.Now().Format("2006-01-02")
	}
	var masak []models.ProduksiMasak
	DB.Preload("Resep").Where("tanggal = ?", tanggal).Order("id desc").Find(&masak)
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

func GetProduksiMatang(c *fiber.Ctx) error {
	tanggal := c.Query("tanggal")
	if tanggal == "" {
		tanggal = time.Now().Format("2006-01-02")
	}
	var matang []models.ProduksiMatang
	DB.Preload("Barang").Where("tanggal = ?", tanggal).Order("id desc").Find(&matang)
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

func GetPembelianBahan(c *fiber.Ctx) error {
	start := c.Query("start")
	end := c.Query("end")

	var beli []models.PembelianBahan

	query := DB.Preload("Bahan")

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

// HANDLER INVENTORY: TUTUP BUKU & LAPORAN

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

	for _, m := range matangList {
		matangMap[m.BarangID] += m.QtyMatang
		barangMap[m.BarangID] = true
	}

	kirimMap := make(map[uint]int)

	type KirimResult struct {
		BarangID uint
		Total    int
	}

	// Tarik Nota Reguler (PERBAIKAN NAMA TABEL: "nota_details" dan "nota")
	var kirimReg []KirimResult
	DB.Table("nota_details").
		Select("nota_details.barang_id, COALESCE(SUM(nota_details.banyak_kirim), 0) as total").
		Joins("JOIN nota ON nota.id = nota_details.nota_id").
		Where("nota.tanggal_kirim = ?", tgl).
		Group("nota_details.barang_id").
		Scan(&kirimReg)

	for _, kr := range kirimReg {
		kirimMap[kr.BarangID] += kr.Total
		barangMap[kr.BarangID] = true
	}

	// Tarik Nota PO (Pesanan) (PERBAIKAN NAMA TABEL: "nota_pesanans")
	var kirimPO []KirimResult
	DB.Table("nota_pesanan_details").
		Select("nota_pesanan_details.barang_id, COALESCE(SUM(nota_pesanan_details.banyak), 0) as total").
		Joins("JOIN nota_pesanans ON nota_pesanans.id = nota_pesanan_details.nota_pesanan_id").
		Where("nota_pesanans.tanggal_kirim = ? AND nota_pesanan_details.barang_id IS NOT NULL", tgl).
		Group("nota_pesanan_details.barang_id").
		Scan(&kirimPO)

	for _, kp := range kirimPO {
		kirimMap[kp.BarangID] += kp.Total
		barangMap[kp.BarangID] = true
	}

	// Eksekusi Pemotongan Sisa
	for barangID := range barangMap {
		sisa := matangMap[barangID] - kirimMap[barangID]

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

	masakMap := make(map[uint]float64)
	for _, m := range masakList {
		masakMap[m.ResepID] += m.TotalAdonan
	}

	hasilMap := make(map[uint]float64)
	for _, m := range matangList {
		var b models.Barang
		DB.First(&b, m.BarangID)
		if b.ResepID != nil {
			hasilMap[*b.ResepID] += float64(m.QtyMatang) * b.KebutuhanAdonan
		}
	}

	for resepID, modal := range masakMap {
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
		DB.First(&b, rs.BarangID)
		sisaAkhir = append(sisaAkhir, models.SisaLayakJual{
			BarangID: b.ID,
			Barang:   b,
			QtySisa:  rs.TotalSisa,
		})
	}

	DB.Preload("Resep").Where("tanggal = ?", tgl).Find(&jurnal)
	return c.JSON(fiber.Map{"jurnal": jurnal, "sisa": sisaAkhir})
}

// HANDLER INVENTORY: STOCK OPNAME (SIDAK GUDANG)
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

// HANDLER INVENTORY: KONVERSI (TARIK SISA KEMARIN)
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
