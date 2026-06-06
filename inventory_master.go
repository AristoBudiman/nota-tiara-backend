package main

import (
	"backend/models"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

// MASTER BAHAN & PEMBELIAN
//
// GetBahan godoc
// @Summary Ambil Seluruh Master Bahan
// @Description Mengambil daftar master bahan baku gudang, diurutkan berdasarkan urutan tampilan.
// @Tags 05. Master Bahan Baku
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {array} models.Bahan "List data bahan berhasil ditarik"
// @Failure 500 {object} models.ErrorResponse "Internal Server Error"
// @Router /api/bahan [get]
func GetBahan(c *fiber.Ctx) error {
	var bahan []models.Bahan
	if err := DB.Order("urutan asc").Preload("KonversiSatuan").Find(&bahan).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(bahan)
}

// CreateBahan godoc
// @Summary Buat Master Bahan Baru
// @Description Menyimpan data master bahan baku baru ke gudang.
// @Tags 05. Master Bahan Baku
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param payload body models.BahanInput true "Data Bahan utuh"
// @Success 200 {object} models.Bahan "Bahan berhasil dibuat"
// @Failure 400 {object} models.ErrorResponse "Format JSON tidak valid"
// @Failure 500 {object} models.ErrorResponse "Kesalahan eksekusi database"
// @Router /api/bahan [post]
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

// UpdateBahan godoc
// @Summary Update Master Bahan
// @Description Memperbarui profil, satuan, atau harga bahan baku berdasarkan ID.
// @Tags 05. Master Bahan Baku
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "ID Bahan"
// @Param payload body models.BahanInput true "Data update bahan utuh"
// @Success 200 {object} models.MessageResponse "Bahan berhasil diupdate"
// @Failure 404 {object} models.ErrorResponse "Bahan tidak ditemukan"
// @Router /api/bahan/{id} [put]
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

// DeleteBahan godoc
// @Summary Hapus Master Bahan (Soft Delete)
// @Description Menghapus master bahan baku dari daftar aktif.
// @Tags 05. Master Bahan Baku
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "ID Bahan"
// @Success 200 {object} models.MessageResponse "Bahan berhasil dihapus"
// @Failure 500 {object} models.ErrorResponse "Internal Server Error"
// @Router /api/bahan/{id} [delete]
func DeleteBahan(c *fiber.Ctx) error {
	id := c.Params("id")
	if err := DB.Delete(&models.Bahan{}, id).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Bahan berhasil dihapus"})
}

// UBAH URUTAN BAHAN (DRAG & DROP)
//
// UpdateUrutanBahan godoc
// @Summary Update Urutan Tampilan Bahan
// @Description Memperbarui posisi urutan bahan baku untuk tampilan di form.
// @Tags 05. Master Bahan Baku
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param payload body []models.UrutanBahanInput true "Array of object key: id, urutan"
// @Success 200 {object} models.MessageResponse "Urutan bahan berhasil diperbarui"
// @Router /api/bahan/reorder [put]
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
//
// GetResep godoc
// @Summary Ambil Seluruh Master Resep
// @Description Mengambil daftar master resep beserta detail array komponen bahan-bahannya.
// @Tags 06. Master Resep
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {array} models.Resep "List data resep ditarik"
// @Router /api/resep [get]
func GetResep(c *fiber.Ctx) error {
	var resep []models.Resep
	// Preload isi resep beserta nama bahan-bahannya
	if err := DB.Preload("BahanDetail.Bahan").Find(&resep).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(resep)
}

// CreateResep godoc
// @Summary Buat Master Resep Baru
// @Description Menyimpan data master resep baru lengkap dengan komposisi bahannya.
// @Tags 06. Master Resep
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param payload body models.ResepInput true "Data Resep utuh"
// @Success 200 {object} models.MessageResponse "Resep berhasil dibuat"
// @Failure 400 {object} models.ErrorResponse "Format JSON salah"
// @Router /api/resep [post]
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

// UpdateResep godoc
// @Summary Update Master Resep
// @Description Memperbarui data resep dan list komposisi bahan berdasarkan ID resep.
// @Tags 06. Master Resep
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "ID Resep"
// @Param payload body models.ResepInput true "Data Resep utuh"
// @Success 200 {object} models.MessageResponse "Resep berhasil diupdate"
// @Router /api/resep/{id} [put]
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

// DeleteResep godoc
// @Summary Hapus Master Resep (Soft Delete)
// @Description Menghapus master resep dari daftar aktif.
// @Tags 06. Master Resep
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "ID Resep"
// @Success 200 {object} models.MessageResponse "Resep berhasil dihapus"
// @Router /api/resep/{id} [delete]
func DeleteResep(c *fiber.Ctx) error {
	id := c.Params("id")
	// Soft delete resep, bahan detail akan terikat oleh relasi tapi resepnya hilang
	if err := DB.Delete(&models.Resep{}, id).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Resep berhasil dihapus"})
}

// ==========================================
// MASTER RESEP KOMPOSIT
// ==========================================
func GetKomposit(c *fiber.Ctx) error {
	var komposit []models.ResepKomposit
	if err := DB.Preload("Details.Bahan").Find(&komposit).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(komposit)
}

func CreateKomposit(c *fiber.Ctx) error {
	var input struct {
		NamaKomposit string `json:"nama_komposit"`
		Details      []struct {
			BahanID uint    `json:"bahan_id"`
			Rasio   float64 `json:"rasio"`
		} `json:"details"`
	}
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format salah"})
	}
	komposit := models.ResepKomposit{NamaKomposit: input.NamaKomposit}
	for _, d := range input.Details {
		komposit.Details = append(komposit.Details, models.ResepKompositDetail{BahanID: d.BahanID, Rasio: d.Rasio})
	}
	if err := DB.Create(&komposit).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Komposit berhasil dibuat!"})
}

func UpdateKomposit(c *fiber.Ctx) error {
	id := c.Params("id")
	var input struct {
		NamaKomposit string `json:"nama_komposit"`
		Details      []struct {
			BahanID uint    `json:"bahan_id"`
			Rasio   float64 `json:"rasio"`
		} `json:"details"`
	}
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format salah"})
	}
	DB.Where("resep_komposit_id = ?", id).Delete(&models.ResepKompositDetail{})
	var newDetails []models.ResepKompositDetail
	parsedID, _ := strconv.Atoi(id)
	for _, d := range input.Details {
		newDetails = append(newDetails, models.ResepKompositDetail{ResepKompositID: uint(parsedID), BahanID: d.BahanID, Rasio: d.Rasio})
	}
	DB.Create(&newDetails)
	DB.Model(&models.ResepKomposit{}).Where("id = ?", id).Update("nama_komposit", input.NamaKomposit)
	return c.JSON(fiber.Map{"message": "Diupdate!"})
}

func DeleteKomposit(c *fiber.Ctx) error {
	id := c.Params("id")
	// Karena struct ResepKomposit punya DeletedAt, ini otomatis SOFT DELETE!
	DB.Delete(&models.ResepKomposit{}, id)
	return c.JSON(fiber.Map{"message": "Dihapus sementara"})
}

// ==========================================
// PENGATURAN KONVERSI SATUAN
// ==========================================
func CreateKonversiSatuan(c *fiber.Ctx) error {
	var input models.KonversiSatuan
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if err := DB.Create(&input).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Konversi satuan berhasil ditambahkan", "data": input})
}

func DeleteKonversiSatuan(c *fiber.Ctx) error {
	id := c.Params("id")
	if err := DB.Delete(&models.KonversiSatuan{}, id).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Konversi satuan berhasil dihapus"})
}
