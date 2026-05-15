package main

import (
	"backend/models"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

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
func GetBarangs(c *fiber.Ctx) error {
	var barangs []models.Barang
	if err := DB.Preload("Resep").Preload("Kemasan.Bahan").Order("urutan asc, id asc").Find(&barangs).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(barangs)
}

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

	if err := DB.Create(&barang).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Barang dan Kemasan berhasil dibuat!", "id": barang.ID})
}

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
	}

	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	var barang models.Barang
	if err := DB.First(&barang, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Barang tidak ditemukan"})
	}

	DB.Where("barang_id = ?", id).Delete(&models.BarangKemasan{})

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

func DeleteBarang(c *fiber.Ctx) error {
	id := c.Params("id")
	if err := DB.Delete(&models.Barang{}, id).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Barang berhasil dihapus"})
}

// MASTER TOKO
func GetTokos(c *fiber.Ctx) error {
	var tokos []models.Toko
	if err := DB.Find(&tokos).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(tokos)
}

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

func DeleteToko(c *fiber.Ctx) error {
	id := c.Params("id")
	if err := DB.Delete(&models.Toko{}, id).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Toko berhasil dihapus"})
}

// SAMPAH
func GetTrash(c *fiber.Ctx) error {
	var tokoTerhapus []models.Toko
	var barangTerhapus []models.Barang
	var bahanTerhapus []models.Bahan
	var resepTerhapus []models.Resep

	DB.Unscoped().Where("deleted_at IS NOT NULL").Find(&tokoTerhapus)
	DB.Unscoped().Where("deleted_at IS NOT NULL").Find(&barangTerhapus)
	DB.Unscoped().Where("deleted_at IS NOT NULL").Find(&bahanTerhapus)
	DB.Unscoped().Where("deleted_at IS NOT NULL").Find(&resepTerhapus)

	return c.JSON(fiber.Map{
		"tokos":   tokoTerhapus,
		"barangs": barangTerhapus,
		"bahans":  bahanTerhapus,
		"reseps":  resepTerhapus,
	})
}

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
	default:
		return c.Status(400).JSON(fiber.Map{"error": "Jenis data tidak valid"})
	}

	return c.JSON(fiber.Map{"message": "Data berhasil dipulihkan"})
}
