package main

import (
	"backend/models"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

// GetProfilTiara godoc
// @Summary Ambil Profil Perusahaan
// @Description Mengambil data profil Tiara Bakery (Nama dan Alamat) untuk keperluan header nota PDF.
// @Tags 02. Master Profil
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} models.ProfilTiara "Berhasil menarik data profil"
// @Router /api/profil [get]
func GetProfilTiara(c *fiber.Ctx) error {
	var profil models.ProfilTiara
	// Ambil data profil pertama yang ada di database
	if err := DB.First(&profil).Error; err != nil {
		// Jika belum ada data di DB, kirim data default agar tidak error
		return c.JSON(models.ProfilTiara{
			Nama:   "TIARA NOTA",
			Alamat: "Alamat belum diatur",
		})
	}
	return c.JSON(profil)
}

// MASTER BARANG
//
// GetBarangs godoc
// @Summary Ambil Seluruh Master Barang
// @Description Mengambil daftar master barang lengkap dengan relasi resep dan kemasan, diurutkan berdasarkan urutan tampilan.
// @Tags 03. Master Barang
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {array} models.Barang "List data barang berhasil ditarik"
// @Failure 500 {object} map[string]interface{} "Internal Server Error"
// @Router /api/barangs [get]
func GetBarangs(c *fiber.Ctx) error {
	var barangs []models.Barang
	if err := DB.Preload("Resep").Preload("Kemasan.Bahan").Preload("Komposit.ResepKomposit.Details").Order("urutan asc, id asc").Find(&barangs).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(barangs)
}

// UpdateUrutanBarang godoc
// @Summary Update Urutan Tampilan Barang
// @Description Memperbarui posisi/urutan barang untuk tampilan di form pembuatan nota. (Dipanggil saat Drag & Drop di UI Vue)
// @Tags 03. Master Barang
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param payload body []models.UrutanBarangInput true "Array of object dengan key: id, urutan"
// @Success 200 {object} map[string]interface{} "Urutan barang berhasil diperbarui"
// @Failure 400 {object} map[string]interface{} "Format data salah"
// @Router /api/barangs/reorder [put]
func UpdateUrutanBarang(c *fiber.Ctx) error {
	var input []struct {
		ID     uint `json:"id"`
		Urutan int  `json:"urutan"`
	}

	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format data salah"})
	}

	// Loop dan update urutan setiap barang di database
	for _, item := range input {
		DB.Model(&models.Barang{}).Where("id = ?", item.ID).Update("urutan", item.Urutan)
	}

	return c.JSON(fiber.Map{"message": "Urutan barang berhasil diperbarui!"})
}

// CreateBarang godoc
// @Summary Buat Master Barang Baru
// @Description Menyimpan data master barang baru beserta kebutuhan kemasannya (kardus/plastik) jika ada.
// @Tags 03. Master Barang
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param payload body models.BarangInput true "Data Barang utuh"
// @Success 200 {object} map[string]interface{} "Barang dan Kemasan berhasil dibuat"
// @Failure 400 {object} map[string]interface{} "Format JSON tidak valid"
// @Failure 500 {object} map[string]interface{} "Gagal menyimpan ke database"
// @Router /api/barangs [post]
func CreateBarang(c *fiber.Ctx) error {
	var input struct {
		NamaBarang      string  `json:"NamaBarang"`
		HargaDefault    float64 `json:"HargaDefault"`
		ResepID         *uint   `json:"resep_id"`
		MetodeKonversi  string  `json:"metode_konversi"`
		KebutuhanAdonan float64 `json:"kebutuhan_adonan"`
		MasaSimpan      int     `json:"masa_simpan"`
		KemasanDetail   []struct {
			BahanID   uint    `json:"bahan_id"`
			Kebutuhan float64 `json:"kebutuhan"`
		} `json:"kemasan_detail"`
		KompositDetail []struct {
			ResepKompositID uint    `json:"resep_komposit_id"`
			Kebutuhan       float64 `json:"kebutuhan"`
		} `json:"komposit_detail"`
	}

	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	var maxUrutan int
	DB.Model(&models.Barang{}).Select("COALESCE(MAX(urutan), 0)").Row().Scan(&maxUrutan)

	barang := models.Barang{
		NamaBarang:      input.NamaBarang,
		HargaDefault:    input.HargaDefault,
		Urutan:          maxUrutan + 1,
		ResepID:         input.ResepID,
		MetodeKonversi:  input.MetodeKonversi,
		KebutuhanAdonan: input.KebutuhanAdonan,
		MasaSimpan:      input.MasaSimpan,
	}

	for _, k := range input.KemasanDetail {
		barang.Kemasan = append(barang.Kemasan, models.BarangKemasan{
			BahanID:   k.BahanID,
			Kebutuhan: k.Kebutuhan,
		})
	}

	// LOGIKA INSERT KOMPOSIT (Taruh di CreateBarang & UpdateBarang)
	var newKomposit []models.BarangKomposit
	for _, k := range input.KompositDetail {
		newKomposit = append(newKomposit, models.BarangKomposit{
			BarangID:        barang.ID, // Gunakan uint(parsedID) untuk fungsi UpdateBarang
			ResepKompositID: k.ResepKompositID,
			Kebutuhan:       k.Kebutuhan,
		})
	}
	if len(newKomposit) > 0 {
		DB.Create(&newKomposit)
	}

	if err := DB.Create(&barang).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Barang dan Kemasan berhasil dibuat!", "id": barang.ID})
}

// UpdateBarang godoc
// @Summary Update Master Barang
// @Description Memperbarui data master barang beserta list kemasannya berdasarkan ID.
// @Tags 03. Master Barang
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "ID Barang"
// @Param payload body models.BarangInput true "Data Update Barang utuh"
// @Success 200 {object} map[string]interface{} "Barang berhasil diupdate"
// @Failure 400 {object} map[string]interface{} "Format JSON tidak valid"
// @Failure 404 {object} map[string]interface{} "Barang tidak ditemukan"
// @Router /api/barangs/{id} [put]
func UpdateBarang(c *fiber.Ctx) error {
	id := c.Params("id")

	var input struct {
		NamaBarang      string  `json:"NamaBarang"`
		HargaDefault    float64 `json:"HargaDefault"`
		ResepID         *uint   `json:"resep_id"`
		MetodeKonversi  string  `json:"metode_konversi"`
		KebutuhanAdonan float64 `json:"kebutuhan_adonan"`
		MasaSimpan      int     `json:"masa_simpan"`
		KemasanDetail   []struct {
			BahanID   uint    `json:"bahan_id"`
			Kebutuhan float64 `json:"kebutuhan"`
		} `json:"kemasan_detail"`
		KompositDetail []struct {
			ResepKompositID uint    `json:"resep_komposit_id"`
			Kebutuhan       float64 `json:"kebutuhan"`
		} `json:"komposit_detail"`
	}

	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	var barang models.Barang
	if err := DB.First(&barang, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Barang tidak ditemukan"})
	}

	DB.Where("barang_id = ?", id).Delete(&models.BarangKemasan{})
	DB.Where("barang_id = ?", id).Delete(&models.BarangKomposit{})

	var newKemasan []models.BarangKemasan
	parsedID, _ := strconv.Atoi(id)
	for _, k := range input.KemasanDetail {
		newKemasan = append(newKemasan, models.BarangKemasan{
			BarangID:  uint(parsedID),
			BahanID:   k.BahanID,
			Kebutuhan: k.Kebutuhan,
		})
	}
	if len(newKemasan) > 0 {
		DB.Create(&newKemasan)
	}

	// LOGIKA INSERT KOMPOSIT (Taruh di CreateBarang & UpdateBarang)
	var newKomposit []models.BarangKomposit
	for _, k := range input.KompositDetail {
		newKomposit = append(newKomposit, models.BarangKomposit{
			BarangID:        barang.ID, // Gunakan uint(parsedID) untuk fungsi UpdateBarang
			ResepKompositID: k.ResepKompositID,
			Kebutuhan:       k.Kebutuhan,
		})
	}
	if len(newKomposit) > 0 {
		DB.Create(&newKomposit)
	}

	DB.Model(&barang).Updates(map[string]interface{}{
		"nama_barang":      input.NamaBarang,
		"harga_default":    input.HargaDefault,
		"resep_id":         input.ResepID,
		"metode_konversi":  input.MetodeKonversi,
		"kebutuhan_adonan": input.KebutuhanAdonan,
		"masa_simpan":      input.MasaSimpan,
	})

	return c.JSON(fiber.Map{"message": "Barang berhasil diupdate"})
}

// DeleteBarang godoc
// @Summary Hapus Master Barang (Soft Delete)
// @Description Menghapus master barang dari daftar aktif (masuk ke tong sampah).
// @Tags 03. Master Barang
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "ID Barang"
// @Success 200 {object} map[string]interface{} "Barang berhasil dihapus"
// @Failure 500 {object} map[string]interface{} "Internal Server Error"
// @Router /api/barangs/{id} [delete]
func DeleteBarang(c *fiber.Ctx) error {
	id := c.Params("id")
	if err := DB.Delete(&models.Barang{}, id).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Barang berhasil dihapus"})
}

// MASTER TOKO
//
// GetTokos godoc
// @Summary Ambil Seluruh Master Toko
// @Description Mengambil data seluruh mitra toko beserta pengaturan siklus tagihannya.
// @Tags 04. Master Toko
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {array} models.Toko "Berhasil menarik data toko"
// @Failure 500 {object} map[string]interface{} "Internal Server Error"
// @Router /api/tokos [get]
func GetTokos(c *fiber.Ctx) error {
	var tokos []models.Toko
	if err := DB.Find(&tokos).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(tokos)
}

// CreateToko godoc
// @Summary Buat Master Toko Baru
// @Description Menyimpan data mitra toko baru beserta aturan siklus tagihannya.
// @Tags 04. Master Toko
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param payload body models.Toko true "Data Toko Baru"
// @Success 200 {object} models.Toko "Toko berhasil dibuat"
// @Failure 400 {object} map[string]interface{} "Format JSON tidak valid"
// @Failure 500 {object} map[string]interface{} "Gagal menyimpan ke database"
// @Router /api/tokos [post]
func CreateToko(c *fiber.Ctx) error {
	var input models.Toko
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if err := DB.Create(&input).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(input)
}

// UpdateToko godoc
// @Summary Update Master Toko
// @Description Memperbarui profil mitra toko dan aturan siklus tagihannya berdasarkan ID.
// @Tags 04. Master Toko
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "ID Toko"
// @Param payload body models.Toko true "Data Update Toko"
// @Success 200 {object} map[string]interface{} "Toko berhasil diupdate"
// @Failure 400 {object} map[string]interface{} "Format JSON tidak valid"
// @Failure 404 {object} map[string]interface{} "Toko tidak ditemukan"
// @Router /api/tokos/{id} [put]
func UpdateToko(c *fiber.Ctx) error {
	id := c.Params("id")
	var input models.Toko

	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	var toko models.Toko
	if err := DB.First(&toko, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Toko tidak ditemukan"})
	}

	DB.Model(&toko).Updates(map[string]interface{}{
		"nama_toko":           input.NamaToko,
		"no_telp":             input.NoTelp,
		"alamat":              input.Alamat,
		"siklus_kamis_senin":  input.SiklusKamisSenin,
		"siklus_jumat_selasa": input.SiklusJumatSelasa,
		"siklus_sabtu_rabu":   input.SiklusSabtuRabu,
		"is_harian":           input.IsHarian,
		"siklus_dua":          input.SiklusDua,
	})

	return c.JSON(fiber.Map{"message": "Toko berhasil diupdate", "data": toko})
}

// DeleteToko godoc
// @Summary Hapus Master Toko (Soft Delete)
// @Description Menghapus mitra toko dari daftar aktif (masuk ke tong sampah).
// @Tags 04. Master Toko
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "ID Toko"
// @Success 200 {object} map[string]interface{} "Toko berhasil dihapus"
// @Failure 500 {object} map[string]interface{} "Internal Server Error"
// @Router /api/tokos/{id} [delete]
func DeleteToko(c *fiber.Ctx) error {
	id := c.Params("id")
	if err := DB.Delete(&models.Toko{}, id).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Toko berhasil dihapus"})
}

// SAMPAH
//
// GetTrash godoc
// @Summary Ambil Data Tong Sampah
// @Description Mengambil semua data master (toko, barang, bahan, resep) yang berstatus soft-deleted.
// @Tags 07. Master Sampah
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{} "Berhasil menarik data sampah"
// @Router /api/sampah [get]
func GetTrash(c *fiber.Ctx) error {
	var tokoTerhapus []models.Toko
	var barangTerhapus []models.Barang
	var bahanTerhapus []models.Bahan
	var resepTerhapus []models.Resep
	var kompositTerhapus []models.ResepKomposit

	DB.Unscoped().Where("deleted_at IS NOT NULL").Find(&tokoTerhapus)
	DB.Unscoped().Where("deleted_at IS NOT NULL").Find(&barangTerhapus)
	DB.Unscoped().Where("deleted_at IS NOT NULL").Find(&bahanTerhapus)
	DB.Unscoped().Where("deleted_at IS NOT NULL").Find(&resepTerhapus)
	DB.Unscoped().Where("deleted_at IS NOT NULL").Find(&kompositTerhapus)

	return c.JSON(fiber.Map{
		"tokos":     tokoTerhapus,
		"barangs":   barangTerhapus,
		"bahans":    bahanTerhapus,
		"reseps":    resepTerhapus,
		"komposits": kompositTerhapus,
	})
}

// RestoreData godoc
// @Summary Pulihkan Data Terhapus
// @Description Mengembalikan data master yang ada di tong sampah ke daftar aktif. Parameter type bisa berisi: 'toko', 'barang', 'bahan', atau 'resep'.
// @Tags 07. Master Sampah
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param type path string true "Jenis Data (toko/barang/bahan/resep)"
// @Param id path int true "ID Data"
// @Success 200 {object} map[string]interface{} "Data berhasil dipulihkan"
// @Failure 400 {object} map[string]interface{} "Jenis data tidak valid"
// @Router /api/sampah/{type}/{id} [put]
func RestoreData(c *fiber.Ctx) error {
	jenis := c.Params("type") // "toko" atau "barang"
	id := c.Params("id")

	switch jenis {
	case "toko":
		DB.Unscoped().Model(&models.Toko{}).Where("id = ?", id).Update("deleted_at", nil)
	case "barang":
		DB.Unscoped().Model(&models.Barang{}).Where("id = ?", id).Update("deleted_at", nil)
	case "bahan":
		DB.Unscoped().Model(&models.Bahan{}).Where("id = ?", id).Update("deleted_at", nil)
	case "resep":
		DB.Unscoped().Model(&models.Resep{}).Where("id = ?", id).Update("deleted_at", nil)
	case "komposit":
		DB.Unscoped().Model(&models.ResepKomposit{}).Where("id = ?", id).Update("deleted_at", nil)
	default:
		return c.Status(400).JSON(fiber.Map{"error": "Jenis data tidak valid"})
	}

	return c.JSON(fiber.Map{"message": "Data berhasil dipulihkan"})
}
