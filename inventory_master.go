package main

import (
	"backend/models"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

// MASTER BAHAN & PEMBELIAN
func GetBahan(c *fiber.Ctx) error {
	var bahan []models.Bahan
	if err := DB.Order("urutan asc").Find(&bahan).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(bahan)
}

func CreateBahan(c *fiber.Ctx) error {
	var input models.Bahan
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	var maxUrutan int
	DB.Model(&models.Bahan{}).Select("COALESCE(MAX(urutan), 0)").Row().Scan(&maxUrutan)
	input.Urutan = maxUrutan + 1

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

// UBAH URUTAN BAHAN (DRAG & DROP)
func UpdateUrutanBahan(c *fiber.Ctx) error {
	var input []struct {
		ID     uint `json:"id"`
		Urutan int  `json:"urutan"`
	}

	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format data salah"})
	}

	// Loop dan update urutan setiap bahan di database
	for _, item := range input {
		DB.Model(&models.Bahan{}).Where("id = ?", item.ID).Update("urutan", item.Urutan)
	}

	return c.JSON(fiber.Map{"message": "Urutan bahan berhasil diperbarui!"})
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
