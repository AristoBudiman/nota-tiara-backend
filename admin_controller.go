package main

import (
	"backend/models"
	"github.com/gofiber/fiber/v2"
	"golang.org/x/crypto/bcrypt"
)

// CreateAdmin godoc
// @Summary Buat Akun Pengguna Baru
// @Description Endpoint khusus Superadmin untuk membuat akun pengguna (Admin/Sales/Dapur).
// @Tags 01. Authentication
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param payload body map[string]string true "Data user (username, password, role)"
// @Success 200 {object} models.MessageResponse "Akun berhasil dibuat"
// @Router /api/admins [post]
func CreateAdmin(c *fiber.Ctx) error {
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
		RoleID   uint   `json:"role_id"`
	}

	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format data salah"})
	}

	if input.Username == "" || input.Password == "" || input.RoleID == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Username, password, dan role wajib diisi"})
	}

	// Cek apakah username sudah ada
	var existingAdmin models.Admin
	if err := DB.Where("username = ?", input.Username).First(&existingAdmin).Error; err == nil {
		return c.Status(400).JSON(fiber.Map{"error": "Username sudah digunakan"})
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal mengamankan password"})
	}

	admin := models.Admin{
		Username: input.Username,
		Password: string(hashedPassword),
		RoleID:   input.RoleID,
	}

	if err := DB.Create(&admin).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal membuat akun: " + err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Akun berhasil dibuat!"})
}

// UpdateAdmin godoc
// @Summary Update Akun Pengguna
// @Description Endpoint khusus Superadmin untuk mengubah role/password pengguna. Kosongkan password jika tidak ingin diganti.
// @Tags 01. Authentication
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "ID Admin"
// @Param payload body map[string]string true "Data user (username, password, role)"
// @Success 200 {object} models.MessageResponse "Akun berhasil diperbarui"
// @Router /api/admins/{id} [put]
func UpdateAdmin(c *fiber.Ctx) error {
	id := c.Params("id")
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"` // Opsional
		RoleID   uint   `json:"role_id"`
	}

	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format data salah"})
	}

	var admin models.Admin
	if err := DB.Preload("Role").First(&admin, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Akun tidak ditemukan"})
	}

	// Cek username bentrok (opsional tapi baik)
	var existingAdmin models.Admin
	if err := DB.Where("username = ? AND id != ?", input.Username, id).First(&existingAdmin).Error; err == nil {
		return c.Status(400).JSON(fiber.Map{"error": "Username sudah digunakan akun lain"})
	}

	// Proteksi: Superadmin tidak boleh mengubah / menurunkan rolenya sendiri
	currentUserID := c.Locals("admin_id").(uint)
	if admin.ID == currentUserID && input.RoleID != admin.RoleID {
		return c.Status(400).JSON(fiber.Map{"error": "Anda tidak boleh mengubah role Anda sendiri!"})
	}

	// Update field
	admin.Username = input.Username
	admin.RoleID = input.RoleID

	if input.Password != "" {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Gagal mengamankan password"})
		}
		admin.Password = string(hashedPassword)
	}

	if err := DB.Save(&admin).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal memperbarui akun: " + err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Akun berhasil diperbarui!"})
}

// DeleteAdmin godoc
// @Summary Hapus Akun Pengguna
// @Description Endpoint khusus Superadmin untuk menghapus pengguna.
// @Tags 01. Authentication
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "ID Admin"
// @Success 200 {object} models.MessageResponse "Akun dihapus"
// @Router /api/admins/{id} [delete]
func DeleteAdmin(c *fiber.Ctx) error {
	id := c.Params("id")
	
	// Cegah hapus diri sendiri
	currentUserID := c.Locals("admin_id").(uint)
	
	var admin models.Admin
	if err := DB.Preload("Role").First(&admin, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Akun tidak ditemukan"})
	}

	if admin.Role.NamaRole == "Superadmin" {
		return c.Status(400).JSON(fiber.Map{"error": "Akun dengan role Superadmin tidak boleh dihapus melalui aplikasi!"})
	}

	if admin.ID == currentUserID {
		return c.Status(400).JSON(fiber.Map{"error": "Tidak bisa menghapus akun Anda sendiri!"})
	}

	if err := DB.Delete(&admin).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal menghapus akun: " + err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Akun berhasil dihapus!"})
}

// GetProfile godoc
// @Summary Ambil Profil Diri Sendiri
// @Description Endpoint untuk user mengambil data username dan role nya sendiri.
// @Tags 01. Authentication
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/profile [get]
func GetProfile(c *fiber.Ctx) error {
	currentUserID := c.Locals("admin_id").(uint)
	var admin models.Admin
	if err := DB.Preload("Role").First(&admin, currentUserID).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "User tidak ditemukan"})
	}

	return c.JSON(fiber.Map{
		"username": admin.Username,
		"role":     admin.Role.NamaRole,
	})
}

// UpdateProfile godoc
// @Summary Ubah Profil Diri Sendiri
// @Description Endpoint untuk user mengubah username atau password nya sendiri.
// @Tags 01. Authentication
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param payload body map[string]string true "Data user (username, password)"
// @Success 200 {object} models.MessageResponse
// @Router /api/profile [put]
func UpdateProfile(c *fiber.Ctx) error {
	currentUserID := c.Locals("admin_id").(uint)
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format data salah"})
	}

	// Cek bentrok username
	if input.Username != "" {
		var existing models.Admin
		if err := DB.Where("username = ? AND id != ?", input.Username, currentUserID).First(&existing).Error; err == nil {
			return c.Status(400).JSON(fiber.Map{"error": "Username sudah digunakan oleh pengguna lain"})
		}
	}

	var admin models.Admin
	if err := DB.First(&admin, currentUserID).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "User tidak ditemukan"})
	}

	if input.Username != "" {
		admin.Username = input.Username
	}

	if input.Password != "" {
		hashed, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Gagal memproses password"})
		}
		admin.Password = string(hashed)
	}

	if err := DB.Save(&admin).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal menyimpan profil: " + err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Profil Anda berhasil diperbarui! Silakan gunakan kredensial baru saat login."})
}
