package main

import (
	"log"
	"backend/models"

	"gorm.io/gorm"
)

func SeedRBAC(DB *gorm.DB) {
	permissions := []models.Permission{
		{Kode: "app_nota", NamaIzin: "Akses Aplikasi Nota"},
		{Kode: "app_inventory", NamaIzin: "Akses Aplikasi Inventory"},
		{Kode: "app_kas", NamaIzin: "Akses Aplikasi Kas"},
		{Kode: "manage_admin", NamaIzin: "Kelola Akun & Role"},
		{Kode: "manage_master_toko", NamaIzin: "Kelola Master Toko"},
		{Kode: "manage_master_barang", NamaIzin: "Kelola Master Barang"},
		{Kode: "manage_master_bahan", NamaIzin: "Kelola Master Bahan & Satuan"},
		{Kode: "manage_resep", NamaIzin: "Kelola Resep"},
		{Kode: "manage_komposit", NamaIzin: "Kelola Komposit"},
		{Kode: "manage_nota_jual", NamaIzin: "Kelola Nota Penjualan"},
		{Kode: "view_riwayat_nota", NamaIzin: "Lihat Riwayat Nota"},
		{Kode: "manage_nota_pesanan", NamaIzin: "Kelola Nota Pesanan"},
		{Kode: "view_riwayat_pesanan", NamaIzin: "Lihat Riwayat Pesanan"},
		{Kode: "manage_produksi_masak", NamaIzin: "Kelola Produksi Masak"},
		{Kode: "manage_produksi_matang", NamaIzin: "Kelola Produksi Matang"},
		{Kode: "manage_pecah_barang", NamaIzin: "Kelola Pecah Barang"},
		{Kode: "manage_tutup_buku", NamaIzin: "Kelola Tutup Buku"},
		{Kode: "manage_opname", NamaIzin: "Kelola Opname"},
		{Kode: "manage_barang_rusak", NamaIzin: "Kelola Barang Rusak / Afkir"},
		{Kode: "manage_pembelian", NamaIzin: "Kelola Pembelian Bahan"},
		{Kode: "manage_kas", NamaIzin: "Kelola Arus Kas"},
		{Kode: "manage_saklar_kas", NamaIzin: "Kelola Pengaturan Kas"},
		{Kode: "view_catatan_besar", NamaIzin: "Lihat Catatan Besar"},
		{Kode: "view_rangkuman_penjualan", NamaIzin: "Lihat Rangkuman Penjualan"},
		{Kode: "view_rangkuman_pesanan", NamaIzin: "Lihat Rangkuman Pesanan"},
		{Kode: "view_jurnal_dapur", NamaIzin: "Lihat Jurnal Dapur & Sisa Kemarin"},
		{Kode: "view_analisis_aset", NamaIzin: "Lihat Analisis Aset"},
		{Kode: "view_dashboard_sales", NamaIzin: "Lihat Dashboard Sales"},
		{Kode: "manage_sampah", NamaIzin: "Akses Tong Sampah"},
	}

	for _, p := range permissions {
		var existing models.Permission
		if err := DB.Where("kode = ?", p.Kode).First(&existing).Error; err != nil {
			DB.Create(&p)
		}
	}

	// Buat Role Superadmin jika belum ada
	var superRole models.Role
	if err := DB.Where("nama_role = ?", "Superadmin").First(&superRole).Error; err != nil {
		superRole = models.Role{NamaRole: "Superadmin", Deskripsi: "Sistem Admin Tertinggi"}
		DB.Create(&superRole)
	}

	// Hubungkan Superadmin ke SEMUA Permission
	var allPerms []models.Permission
	DB.Find(&allPerms)
	DB.Model(&superRole).Association("Permissions").Replace(allPerms)

	var salesRole models.Role
	if err := DB.Where("nama_role = ?", "Sales").First(&salesRole).Error; err != nil {
		salesRole = models.Role{NamaRole: "Sales", Deskripsi: "Sales & Kasir"}
		DB.Create(&salesRole)
	}

	// Set/Update permission default untuk Sales (selalu dieksekusi agar up-to-date)
	var salesPerms []models.Permission
	DB.Where("kode IN ?", []string{
		"app_nota", "manage_nota_jual", "view_riwayat_nota",
		"view_riwayat_pesanan",
		"view_dashboard_sales",
	}).Find(&salesPerms)
	DB.Model(&salesRole).Association("Permissions").Replace(salesPerms)

	// Migrasi Admin lama (Berdasarkan kolom 'role' dari GORM)
	var legacyAdmins []models.Admin
	DB.Find(&legacyAdmins)
	
	for _, a := range legacyAdmins {
		switch a.LegacyRole {
		case "superadmin":
			DB.Model(&a).UpdateColumns(map[string]interface{}{
				"role_id": superRole.ID,
				"role":    "", // Kosongkan
			})
			log.Println("Migrasi: Admin", a.Username, "dipindah ke RoleID Superadmin")
		case "sales":
			DB.Model(&a).UpdateColumns(map[string]interface{}{
				"role_id": salesRole.ID,
				"role":    "",
			})
			log.Println("Migrasi: Admin", a.Username, "dipindah ke RoleID Sales")
		case "dapur":
			// Kita buat sementara role Dapur dan migrate jika ada
			var dapurRole models.Role
			if err := DB.Where("nama_role = ?", "Dapur").First(&dapurRole).Error; err != nil {
				dapurRole = models.Role{NamaRole: "Dapur", Deskripsi: "Tim Produksi"}
				DB.Create(&dapurRole)
			}
			DB.Model(&a).UpdateColumns(map[string]interface{}{
				"role_id": dapurRole.ID,
				"role":    "",
			})
			log.Println("Migrasi: Admin", a.Username, "dipindah ke RoleID Dapur")
		}
	}
	
	log.Println("✅ Seeding & Migrasi RBAC Selesai!")
}
