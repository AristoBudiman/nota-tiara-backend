package main

import (
	"backend/models"

	"github.com/gofiber/fiber/v2"
)

// GetRoles godoc
func GetRoles(c *fiber.Ctx) error {
	var roles []models.Role
	// Preload permissions
	if err := DB.Preload("Permissions").Find(&roles).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(roles)
}

// GetPermissions godoc
func GetPermissions(c *fiber.Ctx) error {
	var permissions []models.Permission
	if err := DB.Find(&permissions).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(permissions)
}

// CreateRole godoc
func CreateRole(c *fiber.Ctx) error {
	var input struct {
		NamaRole      string   `json:"nama_role"`
		Deskripsi     string   `json:"deskripsi"`
		PermissionIds []uint   `json:"permission_ids"`
	}

	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format data salah"})
	}

	if input.NamaRole == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Nama role wajib diisi"})
	}

	var perms []models.Permission
	if len(input.PermissionIds) > 0 {
		DB.Where("id IN ?", input.PermissionIds).Find(&perms)
	}

	role := models.Role{
		NamaRole:    input.NamaRole,
		Deskripsi:   input.Deskripsi,
		Permissions: perms,
	}

	if err := DB.Create(&role).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal membuat role: " + err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Role berhasil dibuat!", "role": role})
}

// UpdateRole godoc
func UpdateRole(c *fiber.Ctx) error {
	id := c.Params("id")

	var role models.Role
	if err := DB.First(&role, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Role tidak ditemukan"})
	}

	if role.NamaRole == "Superadmin" {
		return c.Status(400).JSON(fiber.Map{"error": "Role Superadmin tidak boleh diubah!"})
	}

	var input struct {
		NamaRole      string   `json:"nama_role"`
		Deskripsi     string   `json:"deskripsi"`
		PermissionIds []uint   `json:"permission_ids"`
	}

	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format data salah"})
	}

	role.NamaRole = input.NamaRole
	role.Deskripsi = input.Deskripsi

	// Update data role
	if err := DB.Save(&role).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal memperbarui role: " + err.Error()})
	}

	// Update permissions via Association (replace)
	var perms []models.Permission
	if len(input.PermissionIds) > 0 {
		DB.Where("id IN ?", input.PermissionIds).Find(&perms)
	}
	DB.Model(&role).Association("Permissions").Replace(perms)

	return c.JSON(fiber.Map{"message": "Role berhasil diperbarui!"})
}

// DeleteRole godoc
func DeleteRole(c *fiber.Ctx) error {
	id := c.Params("id")

	var role models.Role
	if err := DB.First(&role, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Role tidak ditemukan"})
	}

	if role.NamaRole == "Superadmin" {
		return c.Status(400).JSON(fiber.Map{"error": "Role Superadmin tidak boleh dihapus!"})
	}

	// Cek apakah ada admin yang masih memakai role ini
	var count int64
	DB.Model(&models.Admin{}).Where("role_id = ?", role.ID).Count(&count)
	if count > 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Tidak bisa menghapus role. Ada akun yang masih menggunakan role ini."})
	}

	// Hapus relasi many2many terlebih dahulu
	DB.Model(&role).Association("Permissions").Clear()

	// Hapus role
	if err := DB.Delete(&role).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal menghapus role: " + err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Role berhasil dihapus!"})
}
