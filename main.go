package main

import (
	"backend/models"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/golang-jwt/jwt/v4"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB
var jwtSecret []byte

// KONSTANTA ROLE RBAC
const (
	RoleSuperadmin = "superadmin"
	RoleSales      = "sales"
	RoleDapur      = "dapur"
)

// KONEKSI KE DB
func connectDB() {
	err := godotenv.Load()
	if err != nil {
		// Gunakan Println agar aplikasi tetap lanjut menyala menggunakan Env Render
		log.Println("Info: File .env fisik tidak ditemukan, menggunakan Environment Variables dari sistem Cloud.")
	}

	jwtSecret = []byte(os.Getenv("JWT_SECRET"))

	host := os.Getenv("DB_HOST")
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")
	dbname := os.Getenv("DB_NAME")
	port := os.Getenv("DB_PORT")

	// Ambil setting SSL dari .env, kalau kosong anggap disable (untuk development)
	ssl := os.Getenv("DB_SSL")
	if ssl == "" {
		ssl = "disable"
	}

	// Masukkan variabel ssl ke dalam dsn
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		host, user, password, dbname, port, ssl)

	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal("Gagal koneksi ke database: ", err)
	}

	log.Println("Koneksi Database Berhasil (via .env)! ✅")

	DB.AutoMigrate(
		&models.ProfilTiara{},
		&models.Toko{},
		&models.Barang{},
		&models.Nota{},
		&models.NotaDetail{},
		&models.Admin{},
		&models.NotaPesanan{},
		&models.NotaPesananDetail{},
		&models.Bahan{},
		&models.PembelianBahan{},
		&models.Resep{},
		&models.ResepBahan{},
		&models.ProduksiMasak{},
		&models.ProduksiMatang{},
		&models.SisaLayakJual{},
		&models.JurnalEfisiensi{},
		&models.StockOpname{},
		&models.BarangKemasan{},
		&models.BarangRusak{},
		&models.TransaksiKas{},
		&models.PengaturanSistem{},
		&models.AsetSnapshot{},
		&models.NotaPesananDetailKemasan{},
	)
	log.Println("Database & Tabel Berhasil Disiapkan! 🏗️")

	var settingKas models.PengaturanSistem
	if err := DB.Where("key = ?", "ENABLE_KAS_SYNC").First(&settingKas).Error; err != nil {
		// Jika belum ada di database, buat otomatis dengan nilai "false"
		DB.Create(&models.PengaturanSistem{
			Key:   "ENABLE_KAS_SYNC",
			Value: "false",
		})
		log.Println("⚙️ Pengaturan Default ENABLE_KAS_SYNC = false berhasil dibuat!")
	}

	// 1. Cek & Buat Super Admin dari .env
	adminUser := os.Getenv("ADMIN_USER")
	adminPass := os.Getenv("ADMIN_PASS")

	// Pastikan ENV tidak kosong sebelum membuat akun!
	if adminUser != "" && adminPass != "" {
		var adminAccount models.Admin
		if err := DB.Where("username = ?", adminUser).First(&adminAccount).Error; err != nil {
			hashedAdmin, _ := bcrypt.GenerateFromPassword([]byte(adminPass), bcrypt.DefaultCost)
			DB.Create(&models.Admin{
				Username: adminUser,
				Password: string(hashedAdmin),
				Role:     "superadmin",
			})
			log.Println("✅ Akun Super Admin siap!")
		}
	} else {
		log.Println("⚠️ PERINGATAN: ADMIN_USER atau ADMIN_PASS di .env kosong! Tidak membuat akun Superadmin.")
	}

	// 2. Cek & Buat Akun Sales dari .env
	salesUser := os.Getenv("SALES_USER")
	salesPass := os.Getenv("SALES_PASS")

	if salesUser != "" && salesPass != "" {
		var salesAccount models.Admin
		if err := DB.Where("username = ?", salesUser).First(&salesAccount).Error; err != nil {
			hashedSales, _ := bcrypt.GenerateFromPassword([]byte(salesPass), bcrypt.DefaultCost)
			DB.Create(&models.Admin{
				Username: salesUser,
				Password: string(hashedSales),
				Role:     "sales",
			})
			log.Println("✅ Akun Sales siap!")
		}
	} else {
		log.Println("⚠️ PERINGATAN: SALES_USER atau SALES_PASS di .env kosong! Tidak membuat akun Sales.")
	}
}

// LOGIN
func LoginAdmin(c *fiber.Ctx) error {
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Input tidak valid"})
	}

	var admin models.Admin
	if err := DB.Where("username = ?", input.Username).First(&admin).Error; err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "Username atau Password salah"})
	}

	// Cek Password Hash vs Password Input
	if err := bcrypt.CompareHashAndPassword([]byte(admin.Password), []byte(input.Password)); err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "Username atau Password salah"})
	}

	// Jika sukses, buat token JWT
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"admin_id": admin.ID,
		"role":     admin.Role,
		"exp":      time.Now().Add(time.Hour * 24).Unix(), // Berlaku 24 jam
	})

	tokenString, err := token.SignedString(jwtSecret)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal membuat token"})
	}

	return c.JSON(fiber.Map{
		"message": "Login sukses",
		"token":   tokenString,
		"role":    admin.Role,
	})
}

func Protected(c *fiber.Ctx) error {
	authHeader := c.Get("Authorization")
	if authHeader == "" || len(authHeader) < 8 {
		return c.Status(401).JSON(fiber.Map{"error": "Akses ditolak. Token tidak ada."})
	}
	tokenString := authHeader[7:]
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return c.Status(401).JSON(fiber.Map{"error": "Token tidak valid atau sudah kedaluwarsa"})
	}

	// BARU: Ekstrak data diri dan simpan di memori lokal request
	claims := token.Claims.(jwt.MapClaims)
	c.Locals("admin_id", uint(claims["admin_id"].(float64)))
	c.Locals("role", claims["role"].(string))

	return c.Next()
}

// MIDDLEWARE RBAC (SATPAM LAPIS KEDUA)
func RequireRole(allowedRoles ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Ambil role dari c.Locals yang diset oleh fungsi Protected
		userRole := c.Locals("role")
		if userRole == nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Akses ditolak. Identitas Role tidak ditemukan.",
			})
		}

		roleStr := userRole.(string)

		// Cek apakah role user ada di dalam daftar role yang diizinkan
		for _, allowedRole := range allowedRoles {
			if roleStr == allowedRole {
				return c.Next() // Role cocok, silakan masuk!
			}
		}

		// Jika looping selesai dan tidak ada yang cocok
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Akses Ditolak! Role Anda (" + roleStr + ") tidak memiliki izin untuk tindakan ini.",
		})
	}
}

// BUAT NOTA
func GetNextNotaNumber(c *fiber.Ctx) error {
	tokoID := c.Query("toko_id")
	tgl := c.Query("tanggal") // Format: 2026-04-27
	tglStr := strings.ReplaceAll(tgl, "-", "")

	var notaTerakhir models.Nota
	// Cari 1 nota terakhir yang dibuat untuk toko ini (berdasarkan ID terbesar)
	err := DB.Unscoped().Where("toko_id = ?", tokoID).Order("id desc").First(&notaTerakhir).Error

	nextUrutan := 1
	if err == nil && notaTerakhir.NoNota != "" {
		// Asumsi format: NT/20260427/15-0017
		// Kita pisahkan berdasarkan tanda strip "-"
		parts := strings.Split(notaTerakhir.NoNota, "-")
		if len(parts) > 1 {
			// Ambil bagian paling belakang (contoh: "0017")
			lastNumStr := parts[len(parts)-1]
			if lastNum, errParse := strconv.Atoi(lastNumStr); errParse == nil {
				nextUrutan = lastNum + 1 // 17 + 1 = 18
			}
		}
	} else {
		// Fallback jika belum ada nota sama sekali untuk toko ini
		var count int64
		DB.Unscoped().Model(&models.Nota{}).Where("toko_id = ?", tokoID).Count(&count)
		nextUrutan = int(count) + 1
	}

	// Format: NT/20260427/1-0001
	// %04d berarti angka akan diformat menjadi 4 digit (0001)
	noNota := fmt.Sprintf("NT/%s/%s-%04d", tglStr, tokoID, nextUrutan)

	return c.JSON(fiber.Map{"no_nota": noNota})
}

func CreateNota(c *fiber.Ctx) error {
	var input struct {
		NoNota       string `json:"no_nota"`
		TokoID       uint   `json:"toko_id"`
		TanggalKirim string `json:"tanggal_kirim"`
		AssignedTo   uint   `json:"assigned_to"`
		Status       string `json:"status"`
		IsLunas      bool   `json:"is_lunas"`
		Details      []struct {
			BarangID    uint `json:"barang_id"`
			BanyakKirim int  `json:"banyak_kirim"`
		} `json:"details"`
	}

	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format data salah"})
	}

	var toko models.Toko
	if err := DB.First(&toko, input.TokoID).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Toko tidak ditemukan"})
	}

	tgl, _ := time.Parse("2006-01-02", input.TanggalKirim)
	hari := tgl.Weekday()

	// Logika Penentuan Siklus Snapshot
	var siklusAktif string

	if toko.IsHarian {
		siklusAktif = "HARIAN"
	} else if toko.SiklusDua {
		siklusAktif = "SiklusDua"
	} else {
		switch {
		case hari == time.Thursday && toko.SiklusKamisSenin:
			siklusAktif = "SiklusKamisSenin"
		case hari == time.Friday && toko.SiklusJumatSelasa:
			siklusAktif = "SiklusJumatSelasa"
		case hari == time.Saturday && toko.SiklusSabtuRabu:
			siklusAktif = "SiklusSabtuRabu"
		default:
			if toko.SiklusKamisSenin {
				siklusAktif = "SiklusKamisSenin"
			} else if toko.SiklusJumatSelasa {
				siklusAktif = "SiklusJumatSelasa"
			} else if toko.SiklusSabtuRabu {
				siklusAktif = "SiklusSabtuRabu"
			}
		}
	}

	adminID := c.Locals("admin_id").(uint) // Ambil ID dari token
	role := c.Locals("role").(string)      // Ambil role yang sedang login

	// LOGIKA OTOMATIS ASSIGNED
	var assignedTo uint = input.AssignedTo
	if role == "sales" {
		// Jika yang buat sales, dia otomatis jadi penanggung jawab (AssignedTo)
		assignedTo = adminID
	}

	// LOGIKA STATUS AWAL
	statusAwal := "KIRIM"
	if input.Status != "" {
		statusAwal = input.Status
	}

	nota := models.Nota{
		NoNota:           input.NoNota,
		TokoID:           input.TokoID,
		TanggalKirim:     tgl,
		Status:           statusAwal,
		NamaTokoSnapshot: toko.NamaToko,
		SiklusSnapshot:   siklusAktif,
		IsHarianSnapshot: toko.IsHarian,
		CreatedBy:        adminID,
		AssignedTo:       assignedTo,
		IsLunas:          input.IsLunas,
	}

	var totalKirim float64
	for _, d := range input.Details {
		var barang models.Barang
		if err := DB.First(&barang, d.BarangID).Error; err == nil {
			subtotal := float64(d.BanyakKirim) * barang.HargaDefault
			totalKirim += subtotal

			nota.Details = append(nota.Details, models.NotaDetail{
				BarangID:           d.BarangID,
				NamaBarangSnapshot: barang.NamaBarang,
				BanyakKirim:        d.BanyakKirim,
				HargaJual:          barang.HargaDefault,
				HargaKirim:         subtotal,
			})
		}
	}

	nota.JumlahKirim = totalKirim
	nota.TotalBayar = totalKirim

	if err := DB.Create(&nota).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// ==========================================
	// FULL SYNC KAS: CREATE NOTA LUNAS
	// ==========================================
	var settingKas models.PengaturanSistem
	DB.Where("key = ?", "ENABLE_KAS_SYNC").First(&settingKas)
	if settingKas.Value == "true" {
		if input.IsLunas {
			DB.Create(&models.TransaksiKas{
				Tanggal:    time.Now(),
				Kategori:   "REGULER",
				Jenis:      "MASUK",
				Nominal:    totalKirim,
				Keterangan: fmt.Sprintf("Pelunasan Reguler - %s (Toko: %s)", nota.NoNota, toko.NamaToko),
				NoNotaRef:  nota.NoNota,
				CreatedBy:  adminID,
			})
		}
	}
	return c.JSON(fiber.Map{"message": "Nota berhasil dibuat!", "id": nota.ID})
}

func UpdateNota(c *fiber.Ctx) error {
	id := c.Params("id")
	var input struct {
		AssignedTo   uint    `json:"assigned_to"`
		Status       string  `json:"status"`
		IsLunas      bool    `json:"is_lunas"`
		TotalDiskon  float64 `json:"total_diskon"`
		TotalVoucher float64 `json:"total_voucher"`
		Details      []struct {
			ID          uint    `json:"id"`
			BarangID    uint    `json:"barang_id"`
			BanyakKirim int     `json:"banyak_kirim"` // <--- 1. TAMBAHKAN PENANGKAP QTY KIRIM
			BanyakRetur int     `json:"banyak_retur"`
			HargaJual   float64 `json:"harga_jual"`
		} `json:"details"`
	}

	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	for _, d := range input.Details {
		hRetur := float64(d.BanyakRetur) * d.HargaJual
		hKirim := float64(d.BanyakKirim) * d.HargaJual // <--- 2. HITUNG ULANG HARGA KIRIM

		if d.ID != 0 {
			// Kasus 1: Detail sudah ada, update KIRIM dan RETUR
			DB.Model(&models.NotaDetail{}).Where("id = ?", d.ID).Updates(map[string]interface{}{
				"banyak_kirim": d.BanyakKirim, // <--- 3. SIMPAN QTY KIRIM BARU
				"harga_kirim":  hKirim,        // <--- 4. SIMPAN HARGA KIRIM BARU
				"banyak_retur": d.BanyakRetur,
				"harga_retur":  hRetur,
			})
		} else if d.BanyakRetur > 0 || d.BanyakKirim > 0 {
			// Kasus 2: Tambah baris baru jika ada isian kirim/retur baru
			var barang models.Barang
			DB.First(&barang, d.BarangID)

			parsedID, err := strconv.Atoi(id)
			if err != nil {
				return c.Status(400).JSON(fiber.Map{"error": "ID Nota tidak valid"})
			}

			newDetail := models.NotaDetail{
				NotaID:             uint(parsedID),
				BarangID:           d.BarangID,
				NamaBarangSnapshot: barang.NamaBarang,
				BanyakKirim:        d.BanyakKirim, // <--- PAKAI INPUT VUE
				HargaJual:          d.HargaJual,
				HargaKirim:         hKirim, // <--- PAKAI HITUNGAN BARU
				BanyakRetur:        d.BanyakRetur,
				HargaRetur:         hRetur,
			}
			DB.Create(&newDetail)
		}
	}

	// Hitung ulang total Kirim & Retur untuk Header Nota
	var totalKirim, totalRetur float64
	DB.Model(&models.NotaDetail{}).Where("nota_id = ?", id).Select("COALESCE(SUM(harga_kirim), 0)").Row().Scan(&totalKirim)
	DB.Model(&models.NotaDetail{}).Where("nota_id = ?", id).Select("COALESCE(SUM(harga_retur), 0)").Row().Scan(&totalRetur)

	// LOGIKA UANG RIIL
	totalBayarAkhir := totalKirim - totalRetur - input.TotalDiskon - input.TotalVoucher

	DB.Model(&models.Nota{}).Where("id = ?", id).Updates(map[string]interface{}{
		"jumlah_kirim":  totalKirim, // <--- 5. WAJIB UPDATE TOTAL KIRIM DI HEADER
		"jumlah_retur":  totalRetur,
		"total_diskon":  input.TotalDiskon,
		"total_voucher": input.TotalVoucher,
		"total_bayar":   totalBayarAkhir,
		"assigned_to":   input.AssignedTo,
		"status":        input.Status,
		"is_lunas":      input.IsLunas,
	})

	// ==========================================
	// FULL SYNC KAS: UPDATE NOTA REGULER
	// ==========================================
	var settingKas models.PengaturanSistem
	DB.Where("key = ?", "ENABLE_KAS_SYNC").First(&settingKas)

	if settingKas.Value == "true" {
		var notaLama models.Nota
		DB.First(&notaLama, id)

		adminID := c.Locals("admin_id").(uint)
		var kasReguler models.TransaksiKas
		errKas := DB.Where("no_nota_ref = ? AND kategori = 'REGULER'", notaLama.NoNota).First(&kasReguler).Error

		if input.IsLunas {
			ket := fmt.Sprintf("Pelunasan Reguler - %s (Toko: %s)", notaLama.NoNota, notaLama.NamaTokoSnapshot)
			if errKas == nil {
				// Sudah ada kasnya, UPDATE nominalnya (Bisa jadi ada tambahan retur/diskon)
				DB.Model(&kasReguler).Updates(map[string]interface{}{
					"nominal":    totalBayarAkhir,
					"keterangan": ket,
				})
			} else {
				// Belum ada, CREATE kas masuk
				DB.Create(&models.TransaksiKas{
					Tanggal:    time.Now(),
					Kategori:   "REGULER",
					Jenis:      "MASUK",
					Nominal:    totalBayarAkhir,
					Keterangan: ket,
					NoNotaRef:  notaLama.NoNota,
					CreatedBy:  adminID,
				})
			}
		} else {
			// Jika TIDAK LUNAS (atau Batal Lunas), HAPUS KAS JIKA ADA!
			if errKas == nil {
				DB.Unscoped().Delete(&kasReguler)
			}
		}
	}

	return c.JSON(fiber.Map{"message": "Nota dan Qty Kirim berhasil diupdate!"})
}

// Batalkan Nota Reguler (Soft Delete & Tarik Kas)
func BatalkanNota(c *fiber.Ctx) error {
	id := c.Params("id")
	tx := DB.Begin()

	var nota models.Nota
	if err := tx.First(&nota, id).Error; err != nil {
		tx.Rollback()
		return c.Status(404).JSON(fiber.Map{"error": "Nota tidak ditemukan"})
	}

	// 1. Ubah status jadi DIBATALKAN
	if err := tx.Model(&nota).Update("status", "DIBATALKAN").Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 2. Tarik uang kembali dari Brankas (Otomatis hapus Kas)
	tx.Unscoped().Where("no_nota_ref = ?", nota.NoNota).Delete(&models.TransaksiKas{})

	tx.Commit()
	return c.JSON(fiber.Map{"message": "Nota berhasil dibatalkan dan Kas ditarik kembali!"})
}

// PULIHKAN NOTA REGULER
func PulihkanNota(c *fiber.Ctx) error {
	id := c.Params("id")
	tx := DB.Begin()

	var nota models.Nota
	if err := tx.First(&nota, id).Error; err != nil {
		tx.Rollback()
		return c.Status(404).JSON(fiber.Map{"error": "Nota tidak ditemukan"})
	}

	// 1. Kembalikan status menjadi aktif (KIRIM)
	if err := tx.Model(&nota).Update("status", "KIRIM").Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 2. Kembalikan Kas (Uang Masuk Lagi) Jika Nota Lunas
	var settingKas models.PengaturanSistem
	tx.Where("key = ?", "ENABLE_KAS_SYNC").First(&settingKas)

	if settingKas.Value == "true" && nota.IsLunas {
		adminID := c.Locals("admin_id").(uint)
		tx.Create(&models.TransaksiKas{
			Tanggal:    time.Now(),
			Kategori:   "REGULER",
			Jenis:      "MASUK",
			Nominal:    nota.TotalBayar, // Masukkan nilai akhir nota
			Keterangan: fmt.Sprintf("Pelunasan Reguler - %s (Toko: %s) [DIPULIHKAN]", nota.NoNota, nota.NamaTokoSnapshot),
			NoNotaRef:  nota.NoNota,
			CreatedBy:  adminID,
		})
	}

	tx.Commit()
	return c.JSON(fiber.Map{"message": "Nota berhasil dipulihkan!"})
}

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

// CATATAN BESAR
func GetCatatanBesar(c *fiber.Ctx) error {
	siklus := c.Query("siklus")
	tanggal := c.Query("tanggal")

	if tanggal == "" {
		tanggal = time.Now().Format("2006-01-02")
	}

	// Filter dinamis: Jika siklus kosong (misal hari Minggu), HANYA cari toko harian
	siklusFilter := "nota.siklus_snapshot = 'HARIAN'"
	if siklus != "" {
		if siklus == "SiklusJumatSelasa" {
			// Jika memilih Selasa/Jumat, tarik data JumatSelasa DAN SiklusDua
			siklusFilter = "(nota.siklus_snapshot = 'SiklusJumatSelasa' OR nota.siklus_snapshot = 'SiklusDua' OR nota.siklus_snapshot = 'HARIAN')"
		} else {
			siklusFilter = fmt.Sprintf("(nota.siklus_snapshot = '%s' OR nota.siklus_snapshot = 'HARIAN')", siklus)
		}
	}

	var results []struct {
		NamaBarang string  `json:"nama_barang"`
		NamaToko   string  `json:"nama_toko"`
		Siklus     string  `json:"siklus"`
		IsHarian   bool    `json:"is_harian"` // Agar Vue tahu opacity-nya
		QtyKirim   int     `json:"qty_kirim"`
		QtyRetur   int     `json:"qty_retur"`
		HargaKirim float64 `json:"harga_kirim"`
		HargaRetur float64 `json:"harga_retur"`
	}

	// INI YANG TADI NGGAK SENGAJA TERHAPUS! KITA KEMBALIKAN!
	kirimDateExpr := `CAST(
		CASE 
			WHEN nota.siklus_snapshot = 'HARIAN' THEN nota.tanggal_kirim
			WHEN nota.siklus_snapshot = 'SiklusDua' THEN 
				CASE 
					WHEN EXTRACT(DOW FROM nota.tanggal_kirim) IN (1,2,3) THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '1 days'
					WHEN EXTRACT(DOW FROM nota.tanggal_kirim) IN (4,5,6) THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '4 days'
					ELSE nota.tanggal_kirim
				END
			WHEN nota.siklus_snapshot = 'SiklusKamisSenin' THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '3 days'
			WHEN nota.siklus_snapshot = 'SiklusJumatSelasa' THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '4 days'
			WHEN nota.siklus_snapshot = 'SiklusSabtuRabu' THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '5 days'
			ELSE nota.tanggal_kirim
		END AS DATE)`

	returDateExpr := `CAST(
		CASE 
			WHEN nota.siklus_snapshot = 'HARIAN' THEN 
				CASE EXTRACT(ISODOW FROM nota.tanggal_kirim)
					WHEN 1 THEN DATE_TRUNC('week', nota.tanggal_kirim) - INTERVAL '4 days'
					WHEN 2 THEN DATE_TRUNC('week', nota.tanggal_kirim) - INTERVAL '3 days'
					WHEN 3 THEN DATE_TRUNC('week', nota.tanggal_kirim) - INTERVAL '2 days'
					WHEN 4 THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '0 days'
					WHEN 5 THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '1 days'
					WHEN 6 THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '2 days'
					WHEN 7 THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '2 days'
					ELSE nota.tanggal_kirim
				END
			WHEN nota.siklus_snapshot = 'SiklusDua' THEN 
				CASE 
					WHEN EXTRACT(DOW FROM nota.tanggal_kirim) IN (1,2,3) THEN DATE_TRUNC('week', nota.tanggal_kirim) - INTERVAL '3 days'
					WHEN EXTRACT(DOW FROM nota.tanggal_kirim) IN (4,5,6) THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '1 days'
					ELSE nota.tanggal_kirim
				END
			WHEN nota.siklus_snapshot = 'SiklusKamisSenin' THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '7 days'
			WHEN nota.siklus_snapshot = 'SiklusJumatSelasa' THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '8 days'
			WHEN nota.siklus_snapshot = 'SiklusSabtuRabu' THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '9 days'
			ELSE nota.tanggal_kirim + INTERVAL '4 days'
		END AS DATE)`

	// Query cerdas: Tarik kerangka 30 hari ke belakang agar tabel stabil,
	// tapi nilai SUM dikunci ketat HANYA pada tanggal filter.
	query := fmt.Sprintf(`
		SELECT 
			barangs.nama_barang, 
			tokos.nama_toko,
			MAX(nota.siklus_snapshot) as siklus,
			bool_or(nota.is_harian_snapshot) as is_harian, 
			COALESCE(SUM(CASE WHEN %s = CAST(? AS DATE) THEN nota_details.banyak_kirim ELSE 0 END), 0) as qty_kirim, 
			COALESCE(SUM(CASE WHEN %s = CAST(? AS DATE) THEN nota_details.banyak_retur ELSE 0 END), 0) as qty_retur, 
			COALESCE(SUM(CASE WHEN %s = CAST(? AS DATE) THEN nota_details.harga_kirim ELSE 0 END), 0) as harga_kirim,
			COALESCE(SUM(CASE WHEN %s = CAST(? AS DATE) THEN nota_details.harga_retur ELSE 0 END), 0) as harga_retur
		FROM nota_details
		JOIN nota ON nota.id = nota_details.nota_id
		JOIN tokos ON tokos.id = nota.toko_id
		JOIN barangs ON barangs.id = nota_details.barang_id
		WHERE 
			%s 
		  AND 
		    nota.tanggal_kirim >= CAST(? AS DATE) - INTERVAL '30 days'
		  AND nota.status != 'DIBATALKAN'
		GROUP BY barangs.nama_barang, tokos.nama_toko
	`, kirimDateExpr, returDateExpr, kirimDateExpr, returDateExpr, siklusFilter)

	// Melempar 5 parameter tanggal
	err := DB.Raw(query, tanggal, tanggal, tanggal, tanggal, tanggal).Scan(&results).Error

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(results)
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

// RIWAYAT NOTA
func GetNotas(c *fiber.Ctx) error {
	var notas []models.Nota
	if err := DB.Preload("Toko").Preload("Details").Preload("Details.Barang").Order("id desc").Find(&notas).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	} // Gunakan "id desc" agar nota yang baru dibuat muncul paling atas
	return c.JSON(notas)
}

func GetNotaByID(c *fiber.Ctx) error {
	id := c.Params("id")
	var nota models.Nota
	if err := DB.Preload("Toko").Preload("Details", func(db *gorm.DB) *gorm.DB {
		return db.Order("nota_details.id ASC")
	}).First(&nota, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Nota tidak ditemukan"})
	}
	return c.JSON(nota)
}

// RANGKUMAN (Logika Anchor Day / Hari Jangkar Mutlak)
func GetRangkuman(c *fiber.Ctx) error {
	start := c.Query("start")
	end := c.Query("end")

	if start == "" || end == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Tanggal start dan end wajib diisi"})
	}

	startDate, _ := time.Parse("2006-01-02", start)
	endDate, _ := time.Parse("2006-01-02", end)

	// 1. AMBIL SEMUA TOKO
	var semuaToko []models.Toko
	DB.Unscoped().Find(&semuaToko)

	rekapMap := make(map[uint]*models.RekapToko)
	for _, t := range semuaToko {
		rekapMap[t.ID] = &models.RekapToko{ID: t.ID, Nama: t.NamaToko, Kirim: 0, Retur: 0, Diskon: 0, Pendapatan: 0, Persentase: 0}
	}

	kirimDateExpr := `CAST(
		CASE 
			WHEN nota.siklus_snapshot = 'HARIAN' THEN nota.tanggal_kirim
			WHEN nota.siklus_snapshot = 'SiklusDua' THEN 
				CASE 
					WHEN EXTRACT(DOW FROM nota.tanggal_kirim) IN (1,2,3) THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '1 days'
					WHEN EXTRACT(DOW FROM nota.tanggal_kirim) IN (4,5,6) THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '4 days'
					ELSE nota.tanggal_kirim
				END
			WHEN nota.siklus_snapshot = 'SiklusKamisSenin' THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '3 days'
			WHEN nota.siklus_snapshot = 'SiklusJumatSelasa' THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '4 days'
			WHEN nota.siklus_snapshot = 'SiklusSabtuRabu' THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '5 days'
			ELSE nota.tanggal_kirim
		END AS DATE)`

	returDateExpr := `CAST(
		CASE 
			WHEN nota.siklus_snapshot = 'HARIAN' THEN 
				CASE EXTRACT(DOW FROM nota.tanggal_kirim)
					WHEN 1 THEN DATE_TRUNC('week', nota.tanggal_kirim) - INTERVAL '4 days'
					WHEN 2 THEN DATE_TRUNC('week', nota.tanggal_kirim) - INTERVAL '3 days'
					WHEN 3 THEN DATE_TRUNC('week', nota.tanggal_kirim) - INTERVAL '2 days'
					WHEN 4 THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '0 days'
					WHEN 5 THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '1 days'
					WHEN 6 THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '2 days'
					WHEN 0 THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '2 days' 
					ELSE nota.tanggal_kirim
				END
			WHEN nota.siklus_snapshot = 'SiklusDua' THEN 
				CASE 
					WHEN EXTRACT(DOW FROM nota.tanggal_kirim) IN (1,2,3) THEN DATE_TRUNC('week', nota.tanggal_kirim) - INTERVAL '3 days'
					WHEN EXTRACT(DOW FROM nota.tanggal_kirim) IN (4,5,6) THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '1 days'
					ELSE nota.tanggal_kirim
				END
			WHEN nota.siklus_snapshot = 'SiklusKamisSenin' THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '7 days'
			WHEN nota.siklus_snapshot = 'SiklusJumatSelasa' THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '8 days'
			WHEN nota.siklus_snapshot = 'SiklusSabtuRabu' THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '9 days'
			ELSE nota.tanggal_kirim + INTERVAL '4 days'
		END AS DATE)`

	var rawResults []struct {
		ID     uint
		Nama   string
		Kirim  float64
		Retur  float64
		Diskon float64
	}

	// 3. EKSEKUSI SQL TOKO (Menambahkan penarikan Diskon + Voucher)
	queryToko := fmt.Sprintf(`
		SELECT 
			toko_id as id,
			MAX(nama_toko_snapshot) as nama,
			COALESCE(SUM(CASE WHEN %[1]s >= CAST(? AS DATE) AND %[1]s <= CAST(? AS DATE) THEN jumlah_kirim ELSE 0 END), 0) as kirim,
			COALESCE(SUM(CASE WHEN %[2]s >= CAST(? AS DATE) AND %[2]s <= CAST(? AS DATE) THEN jumlah_retur ELSE 0 END), 0) as retur,
			COALESCE(SUM(CASE WHEN %[1]s >= CAST(? AS DATE) AND %[1]s <= CAST(? AS DATE) THEN (total_diskon + total_voucher) ELSE 0 END), 0) as diskon
		FROM nota
		WHERE
			( 
				(%[1]s >= CAST(? AS DATE) AND %[1]s <= CAST(? AS DATE))
				OR 
				(%[2]s >= CAST(? AS DATE) AND %[2]s <= CAST(? AS DATE))
			)
			AND nota.status != 'DIBATALKAN'
		GROUP BY toko_id
	`, kirimDateExpr, returDateExpr)

	DB.Raw(queryToko, startDate, endDate, startDate, endDate, startDate, endDate, startDate, endDate, startDate, endDate).Scan(&rawResults)

	var totalKirim, totalRetur, totalDiskon float64

	for _, r := range rawResults {
		if val, exists := rekapMap[r.ID]; exists {
			val.Kirim = r.Kirim
			val.Retur = r.Retur
			val.Diskon = r.Diskon
			val.Pendapatan = r.Kirim - r.Retur - r.Diskon // <--- Potong dengan diskon riil

			// LOGIKA BISNIS: Jika ada kirim, hitung normal. Jika tidak ada kirim tapi ada retur, SET 100%!
			if r.Kirim > 0 {
				val.Persentase = (r.Retur / r.Kirim) * 100
			} else if r.Retur > 0 {
				val.Persentase = 100
			} else {
				val.Persentase = 0
			}

			if r.Nama != "" {
				val.Nama = r.Nama
			}
		}
		totalKirim += r.Kirim
		totalRetur += r.Retur
		totalDiskon += r.Diskon
	}

	var perToko []models.RekapToko
	for _, r := range rekapMap {
		perToko = append(perToko, *r)
	}

	sort.Slice(perToko, func(i, j int) bool { return perToko[i].Pendapatan > perToko[j].Pendapatan })

	totalPersentase := 0.0
	if totalKirim > 0 {
		totalPersentase = (totalRetur / totalKirim) * 100
	} else if totalRetur > 0 {
		totalPersentase = 100
	}

	var rawBarang []struct {
		Nama  string
		Kirim float64
		Retur float64
	}

	// 6. EKSEKUSI SQL BARANG
	queryBarang := fmt.Sprintf(`
		SELECT 
			MAX(nota_details.nama_barang_snapshot) as nama,
			COALESCE(SUM(CASE WHEN %s >= CAST(? AS DATE) AND %s <= CAST(? AS DATE) THEN nota_details.banyak_kirim ELSE 0 END), 0) as kirim,
			COALESCE(SUM(CASE WHEN %s >= CAST(? AS DATE) AND %s <= CAST(? AS DATE) THEN nota_details.banyak_retur ELSE 0 END), 0) as retur
		FROM nota_details
		JOIN nota ON nota.id = nota_details.nota_id
		WHERE
			(
				(%s >= CAST(? AS DATE) AND %s <= CAST(? AS DATE))
				OR 
				(%s >= CAST(? AS DATE) AND %s <= CAST(? AS DATE))
			)
			AND nota.status != 'DIBATALKAN'
		GROUP BY nota_details.barang_id
	`, kirimDateExpr, kirimDateExpr, returDateExpr, returDateExpr, kirimDateExpr, kirimDateExpr, returDateExpr, returDateExpr)

	DB.Raw(queryBarang, startDate, endDate, startDate, endDate, startDate, endDate, startDate, endDate).Scan(&rawBarang)

	var perBarang []models.RekapBarang
	for _, b := range rawBarang {
		if b.Kirim == 0 && b.Retur == 0 {
			continue
		}

		persen := 0.0
		if b.Kirim > 0 {
			persen = (b.Retur / b.Kirim) * 100
		} else if b.Retur > 0 {
			persen = 100
		}

		perBarang = append(perBarang, models.RekapBarang{
			Nama:       b.Nama,
			QtyKirim:   b.Kirim,
			QtyRetur:   b.Retur,
			QtyLaku:    b.Kirim - b.Retur,
			Persentase: persen,
		})
	}

	sort.Slice(perBarang, func(i, j int) bool { return perBarang[i].QtyLaku > perBarang[j].QtyLaku })

	return c.JSON(models.RangkumanResponse{
		Kirim:      totalKirim,
		Retur:      totalRetur,
		Diskon:     totalDiskon,
		Pendapatan: totalKirim - totalRetur - totalDiskon, // <--- Hasil Kas Riil
		Persentase: totalPersentase,
		PerToko:    perToko,
		PerBarang:  perBarang,
	})
}

func GetRangkumanPerToko(c *fiber.Ctx) error {
	start := c.Query("start")
	end := c.Query("end")
	tokoID := c.Query("toko_id")

	if tokoID == "" || tokoID == "null" || tokoID == "undefined" {
		return c.Status(400).JSON(fiber.Map{"error": "ID Toko tidak boleh kosong"})
	}

	var hasil []struct {
		NamaBarang string  `json:"nama_barang"`
		TotalKirim int     `json:"total_kirim"`
		TotalRetur int     `json:"total_retur"`
		TotalLaku  int     `json:"total_laku"`
		Persentase float64 `json:"persentase"`
	}

	kirimDateExpr := `CAST(
		CASE 
			WHEN nota.siklus_snapshot = 'HARIAN' THEN nota.tanggal_kirim
			WHEN nota.siklus_snapshot = 'SiklusDua' THEN 
				CASE 
					WHEN EXTRACT(DOW FROM nota.tanggal_kirim) IN (1,2,3) THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '1 days'
					WHEN EXTRACT(DOW FROM nota.tanggal_kirim) IN (4,5,6) THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '4 days'
					ELSE nota.tanggal_kirim
				END
			WHEN nota.siklus_snapshot = 'SiklusKamisSenin' THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '3 days'
			WHEN nota.siklus_snapshot = 'SiklusJumatSelasa' THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '4 days'
			WHEN nota.siklus_snapshot = 'SiklusSabtuRabu' THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '5 days'
			ELSE nota.tanggal_kirim
		END AS DATE)`

	returDateExpr := `CAST(
		CASE 
			WHEN nota.siklus_snapshot = 'HARIAN' THEN 
				CASE EXTRACT(DOW FROM nota.tanggal_kirim)
					WHEN 1 THEN DATE_TRUNC('week', nota.tanggal_kirim) - INTERVAL '4 days'
					WHEN 2 THEN DATE_TRUNC('week', nota.tanggal_kirim) - INTERVAL '3 days'
					WHEN 3 THEN DATE_TRUNC('week', nota.tanggal_kirim) - INTERVAL '2 days'
					WHEN 4 THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '0 days'
					WHEN 5 THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '1 days'
					WHEN 6 THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '2 days'
					WHEN 0 THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '2 days'
					ELSE nota.tanggal_kirim
				END
			WHEN nota.siklus_snapshot = 'SiklusDua' THEN 
				CASE 
					WHEN EXTRACT(DOW FROM nota.tanggal_kirim) IN (1,2,3) THEN DATE_TRUNC('week', nota.tanggal_kirim) - INTERVAL '3 days'
					WHEN EXTRACT(DOW FROM nota.tanggal_kirim) IN (4,5,6) THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '1 days'
					ELSE nota.tanggal_kirim
				END
			WHEN nota.siklus_snapshot = 'SiklusKamisSenin' THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '7 days'
			WHEN nota.siklus_snapshot = 'SiklusJumatSelasa' THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '8 days'
			WHEN nota.siklus_snapshot = 'SiklusSabtuRabu' THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '9 days'
			ELSE nota.tanggal_kirim + INTERVAL '4 days'
		END AS DATE)`

	query := fmt.Sprintf(`
		SELECT 
			MAX(nota_details.nama_barang_snapshot) as nama_barang, 
			COALESCE(SUM(CASE WHEN %s >= CAST(? AS DATE) AND %s <= CAST(? AS DATE) THEN nota_details.banyak_kirim ELSE 0 END), 0) as total_kirim, 
			COALESCE(SUM(CASE WHEN %s >= CAST(? AS DATE) AND %s <= CAST(? AS DATE) THEN nota_details.banyak_retur ELSE 0 END), 0) as total_retur
		FROM nota_details
		JOIN nota ON nota.id = nota_details.nota_id
		WHERE 
			((%s >= CAST(? AS DATE) AND %s <= CAST(? AS DATE))
			OR 
			(%s >= CAST(? AS DATE) AND %s <= CAST(? AS DATE)))
		AND nota.toko_id = ?
		AND nota.status != 'DIBATALKAN'
		GROUP BY nota_details.barang_id
	`, kirimDateExpr, kirimDateExpr, returDateExpr, returDateExpr, kirimDateExpr, kirimDateExpr, returDateExpr, returDateExpr)

	DB.Raw(query, start, end, start, end, start, end, start, end, tokoID).Scan(&hasil)

	for i := range hasil {
		hasil[i].TotalLaku = hasil[i].TotalKirim - hasil[i].TotalRetur

		// LOGIKA BISNIS: Kirim 0 tapi Retur > 0 = 100%
		if hasil[i].TotalKirim > 0 {
			hasil[i].Persentase = (float64(hasil[i].TotalRetur) / float64(hasil[i].TotalKirim)) * 100
		} else if hasil[i].TotalRetur > 0 {
			hasil[i].Persentase = 100
		} else {
			hasil[i].Persentase = 0
		}
	}

	sort.Slice(hasil, func(i, j int) bool { return hasil[i].TotalLaku > hasil[j].TotalLaku })

	return c.JSON(hasil)
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

// NOTA PESANAN
func GetNextNotaPesananNumber(c *fiber.Ctx) error {
	tgl := c.Query("tanggal") // 2026-04-30
	tglStr := strings.ReplaceAll(tgl, "-", "")
	tokoID := c.Query("toko_id")

	if tokoID == "" {
		tokoID = "0" // 0 berarti PABRIK
	}

	var poTerakhir models.NotaPesanan
	query := DB.Unscoped().Order("id desc")

	if tokoID == "0" {
		query.Where("toko_id IS NULL").First(&poTerakhir)
	} else {
		query.Where("toko_id = ?", tokoID).First(&poTerakhir)
	}

	nextUrutan := 1
	if poTerakhir.NoNota != "" {
		parts := strings.Split(poTerakhir.NoNota, "-")
		if len(parts) > 1 {
			lastNumStr := parts[len(parts)-1]
			if lastNum, err := strconv.Atoi(lastNumStr); err == nil {
				nextUrutan = lastNum + 1
			}
		}
	} else {
		// Fallback jika belum ada nota sama sekali
		var count int64
		if tokoID == "0" {
			DB.Unscoped().Model(&models.NotaPesanan{}).Where("toko_id IS NULL").Count(&count)
		} else {
			DB.Unscoped().Model(&models.NotaPesanan{}).Where("toko_id = ?", tokoID).Count(&count)
		}
		nextUrutan = int(count) + 1
	}

	// Format: PO/20260430/0-0001 (Pabrik) atau PO/20260430/15-0001 (Mitra)
	noNota := fmt.Sprintf("PO/%s/%s-%04d", tglStr, tokoID, nextUrutan)

	return c.JSON(fiber.Map{"no_nota": noNota})
}

func CreateNotaPesanan(c *fiber.Ctx) error {
	var input struct {
		NoNota           string  `json:"no_nota"`
		NamaPemesan      string  `json:"nama_pemesan"`
		TanggalKirim     string  `json:"tanggal_kirim"`
		JenisPengambilan string  `json:"jenis_pengambilan"`
		TokoID           *uint   `json:"toko_id"`
		AssignedTo       uint    `json:"assigned_to"`
		Status           string  `json:"status"`
		IsLunas          bool    `json:"is_lunas"`
		Ongkir           float64 `json:"ongkir"`
		UangMuka         float64 `json:"uang_muka"`     // <--- BARU: Tangkap DP
		TotalVoucher     float64 `json:"total_voucher"` // <--- BARU: Tangkap Voucher
		Details          []struct {
			BarangID        *uint   `json:"barang_id"`
			NamaBarangBebas string  `json:"nama_barang_bebas"`
			Banyak          int     `json:"banyak"`
			HargaJual       float64 `json:"harga_jual"`
			ResepID         *uint   `json:"resep_id"`
			Gramasi         float64 `json:"gramasi"`
			KemasanDetail   []struct {
				BahanID   uint    `json:"bahan_id"`
				Kebutuhan float64 `json:"kebutuhan"`
			} `json:"kemasan_detail"`
		} `json:"details"`
	}

	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format data salah"})
	}

	tgl, _ := time.Parse("2006-01-02", input.TanggalKirim)
	adminID := c.Locals("admin_id").(uint)

	namaTokoSnapshot := "PABRIK"
	if input.JenisPengambilan == "MITRA" && input.TokoID != nil {
		var toko models.Toko
		if err := DB.First(&toko, *input.TokoID).Error; err == nil {
			namaTokoSnapshot = toko.NamaToko
		}
	}

	pesanan := models.NotaPesanan{
		NoNota:           input.NoNota,
		NamaPemesan:      input.NamaPemesan,
		TanggalKirim:     tgl,
		JenisPengambilan: input.JenisPengambilan,
		TokoID:           input.TokoID,
		NamaTokoSnapshot: namaTokoSnapshot,
		CreatedBy:        adminID,
		AssignedTo:       input.AssignedTo,
		Status:           input.Status,
		IsLunas:          input.IsLunas,
	}

	var totalBayar float64
	for _, d := range input.Details {
		subtotal := float64(d.Banyak) * d.HargaJual
		totalBayar += subtotal

		// --- LOGIKA KEMASAN BARU ---
		var kemasanArr []models.NotaPesananDetailKemasan
		for _, k := range d.KemasanDetail {
			kemasanArr = append(kemasanArr, models.NotaPesananDetailKemasan{
				BahanID:   k.BahanID,
				Kebutuhan: k.Kebutuhan,
			})
		}

		pesanan.Details = append(pesanan.Details, models.NotaPesananDetail{
			BarangID:        d.BarangID,
			NamaBarangBebas: d.NamaBarangBebas,
			Banyak:          d.Banyak,
			HargaJual:       d.HargaJual,
			Subtotal:        subtotal,
			ResepID:         d.ResepID,
			Gramasi:         d.Gramasi,
			KemasanDetail:   kemasanArr,
		})
	}

	// LOGIKA UANG RIIL PO
	pesanan.TotalBayar = totalBayar
	pesanan.Ongkir = input.Ongkir
	pesanan.UangMuka = input.UangMuka
	pesanan.TotalVoucher = input.TotalVoucher
	pesanan.SisaTagihan = totalBayar + input.Ongkir - input.UangMuka - input.TotalVoucher

	if err := DB.Create(&pesanan).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// ==========================================
	// FULL SYNC KAS: CREATE PO BARU
	// ==========================================
	var settingKas models.PengaturanSistem
	DB.Where("key = ?", "ENABLE_KAS_SYNC").First(&settingKas)

	if settingKas.Value == "true" {

		// 1. Catat DP Jika Ada
		if pesanan.UangMuka > 0 {
			DB.Create(&models.TransaksiKas{
				Tanggal:    time.Now(),
				Kategori:   "PESANAN",
				Jenis:      "MASUK",
				Nominal:    pesanan.UangMuka,
				Keterangan: fmt.Sprintf("DP Pesanan - %s (Pemesan: %s)", pesanan.NoNota, pesanan.NamaPemesan),
				NoNotaRef:  pesanan.NoNota,
				CreatedBy:  adminID,
			})
		}

		// 2. Catat Pelunasan Sisa PO jika langsung dilunasi
		if pesanan.IsLunas && pesanan.SisaTagihan > 0 {
			DB.Create(&models.TransaksiKas{
				Tanggal:    time.Now(),
				Kategori:   "PESANAN",
				Jenis:      "MASUK",
				Nominal:    pesanan.SisaTagihan,
				Keterangan: fmt.Sprintf("Pelunasan Sisa PO - %s (Pemesan: %s)", pesanan.NoNota, pesanan.NamaPemesan),
				NoNotaRef:  pesanan.NoNota,
				CreatedBy:  adminID,
			})
		}
	}

	return c.JSON(fiber.Map{"message": "Pesanan berhasil dibuat!", "id": pesanan.ID})
}

func GetCatatanPesanan(c *fiber.Ctx) error {
	tgl := c.Query("tanggal") // Cukup kirim 1 tanggal (hari H)

	var results []struct {
		NamaBarangBebas  string  `json:"nama_barang_bebas"`
		NamaTokoSnapshot string  `json:"nama_toko"`
		JenisPengambilan string  `json:"jenis_pengambilan"`
		TotalBanyak      int     `json:"total_banyak"`
		TotalRupiah      float64 `json:"total_rupiah"` // <--- BARU: Tangkap jumlah uang
	}

	// Query rekap berdasarkan hari H pengiriman pesanan
	err := DB.Table("nota_pesanan_details").
		Select("nota_pesanan_details.nama_barang_bebas, nota_pesanans.nama_toko_snapshot, nota_pesanans.jenis_pengambilan, SUM(nota_pesanan_details.banyak) as total_banyak, SUM(nota_pesanan_details.subtotal) as total_rupiah"). // <--- BARU: Tarik Subtotal
		Joins("join nota_pesanans on nota_pesanans.id = nota_pesanan_details.nota_pesanan_id").
		Where("DATE(nota_pesanans.tanggal_kirim) = ?", tgl).
		Group("nota_pesanan_details.nama_barang_bebas, nota_pesanans.nama_toko_snapshot, nota_pesanans.jenis_pengambilan").
		Scan(&results).Error

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(results)
}

// GET PO BY ID
func GetNotaPesananByID(c *fiber.Ctx) error {
	id := c.Params("id")
	var pesanan models.NotaPesanan

	// UBAH BARIS INI AGAR GOLANG MENGIRIM DATA KEMASAN SAAT DI-EDIT:
	if err := DB.Preload("Toko").Preload("Details").Preload("Details.Barang").Preload("Details.KemasanDetail").First(&pesanan, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Pesanan tidak ditemukan"})
	}

	return c.JSON(pesanan)
}

// UPDATE PO
func UpdateNotaPesanan(c *fiber.Ctx) error {
	id := c.Params("id")
	var input struct {
		NamaPemesan      string  `json:"nama_pemesan"`
		TanggalKirim     string  `json:"tanggal_kirim"`
		JenisPengambilan string  `json:"jenis_pengambilan"`
		TokoID           *uint   `json:"toko_id"`
		AssignedTo       uint    `json:"assigned_to"`
		Status           string  `json:"status"`
		IsLunas          bool    `json:"is_lunas"`
		Ongkir           float64 `json:"ongkir"`
		UangMuka         float64 `json:"uang_muka"`     // <--- BARU: Tangkap DP
		TotalVoucher     float64 `json:"total_voucher"` // <--- BARU: Tangkap Voucher
		Details          []struct {
			BarangID        *uint   `json:"barang_id"`
			NamaBarangBebas string  `json:"nama_barang_bebas"`
			Banyak          int     `json:"banyak"`
			HargaJual       float64 `json:"harga_jual"`
			ResepID         *uint   `json:"resep_id"`
			Gramasi         float64 `json:"gramasi"`
			KemasanDetail   []struct {
				BahanID   uint    `json:"bahan_id"`
				Kebutuhan float64 `json:"kebutuhan"`
			} `json:"kemasan_detail"`
		} `json:"details"`
	}

	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format data salah"})
	}

	tgl, _ := time.Parse("2006-01-02", input.TanggalKirim)

	// --- MULAI BLOK REFUND STOK (TAMBAHKAN INI) ---
	var detailLama []models.NotaPesananDetail
	// Tarik data detail lama beserta kemasannya
	DB.Preload("KemasanDetail").Where("nota_pesanan_id = ?", id).Find(&detailLama)

	for _, d := range detailLama {
		// Jika statusnya sudah pernah dipotong oleh Tutup Buku, kembalikan stoknya dulu
		if d.IsKemasanTerpotong {
			for _, k := range d.KemasanDetail {
				totalBalik := float64(d.Banyak) * k.Kebutuhan
				DB.Model(&models.Bahan{}).Where("id = ?", k.BahanID).
					Update("stok", gorm.Expr("stok + ?", totalBalik))
			}
		}
	}

	// 1. HAPUS KEMASAN DULU (ANAKNYA) AGAR TIDAK DIBLOKIR FOREIGN KEY
	DB.Exec("DELETE FROM nota_pesanan_detail_kemasans WHERE nota_pesanan_detail_id IN (SELECT id FROM nota_pesanan_details WHERE nota_pesanan_id = ?)", id)

	// 2. BARU HAPUS DETAIL LAMA (INDUKNYA)
	DB.Where("nota_pesanan_id = ?", id).Delete(&models.NotaPesananDetail{})

	var totalBayar float64
	var newDetails []models.NotaPesananDetail

	for _, d := range input.Details {
		sub := float64(d.Banyak) * d.HargaJual
		totalBayar += sub
		parsedID, _ := strconv.Atoi(id)

		// --- LOGIKA KEMASAN BARU ---
		var kemasanArr []models.NotaPesananDetailKemasan
		for _, k := range d.KemasanDetail {
			kemasanArr = append(kemasanArr, models.NotaPesananDetailKemasan{
				BahanID:   k.BahanID,
				Kebutuhan: k.Kebutuhan,
			})
		}

		newDetails = append(newDetails, models.NotaPesananDetail{
			NotaPesananID:   uint(parsedID),
			BarangID:        d.BarangID,
			NamaBarangBebas: d.NamaBarangBebas,
			Banyak:          d.Banyak,
			HargaJual:       d.HargaJual,
			Subtotal:        sub,
			ResepID:         d.ResepID,
			Gramasi:         d.Gramasi,
			KemasanDetail:   kemasanArr,
		})
	}

	DB.Create(&newDetails)

	// Update Header
	namaTokoSnap := "PABRIK"
	if input.JenisPengambilan == "MITRA" && input.TokoID != nil {
		var t models.Toko
		DB.First(&t, *input.TokoID)
		namaTokoSnap = t.NamaToko
	}

	// HITUNG ULANG SISA TAGIHAN SAAT DI-UPDATE
	sisaTagihan := totalBayar + input.Ongkir - input.UangMuka - input.TotalVoucher

	DB.Model(&models.NotaPesanan{}).Where("id = ?", id).Updates(map[string]interface{}{
		"nama_pemesan":       input.NamaPemesan,
		"tanggal_kirim":      tgl,
		"jenis_pengambilan":  input.JenisPengambilan,
		"toko_id":            input.TokoID,
		"nama_toko_snapshot": namaTokoSnap,
		"assigned_to":        input.AssignedTo,
		"status":             input.Status,
		"is_lunas":           input.IsLunas,
		"total_bayar":        totalBayar,
		"ongkir":             input.Ongkir,
		"uang_muka":          input.UangMuka,     // <--- UPDATE DP
		"total_voucher":      input.TotalVoucher, // <--- UPDATE VOUCHER
		"sisa_tagihan":       sisaTagihan,        // <--- UPDATE SISA
	})

	// ==========================================
	// FULL SYNC KAS: UPDATE PO
	// ==========================================
	var settingKas models.PengaturanSistem
	DB.Where("key = ?", "ENABLE_KAS_SYNC").First(&settingKas)

	if settingKas.Value == "true" {
		var poLama models.NotaPesanan
		DB.First(&poLama, id)
		adminID := c.Locals("admin_id").(uint)

		// 1. SINKRONISASI DP PESANAN
		var kasDP models.TransaksiKas
		errDP := DB.Where("no_nota_ref = ? AND keterangan LIKE 'DP Pesanan%'", poLama.NoNota).First(&kasDP).Error

		if input.UangMuka > 0 {
			ketDP := fmt.Sprintf("DP Pesanan - %s (Pemesan: %s)", poLama.NoNota, input.NamaPemesan)
			if errDP == nil {
				DB.Model(&kasDP).Updates(map[string]interface{}{"nominal": input.UangMuka, "keterangan": ketDP})
			} else {
				DB.Create(&models.TransaksiKas{
					Tanggal: time.Now(), Kategori: "PESANAN", Jenis: "MASUK",
					Nominal: input.UangMuka, Keterangan: ketDP, NoNotaRef: poLama.NoNota, CreatedBy: adminID,
				})
			}
		} else { // Jika DP di-nol-kan, hapus kas DP
			if errDP == nil {
				DB.Unscoped().Delete(&kasDP)
			}
		}

		// 2. SINKRONISASI PELUNASAN SISA TAGIHAN
		var kasSisa models.TransaksiKas
		errSisa := DB.Where("no_nota_ref = ? AND keterangan LIKE 'Pelunasan Sisa PO%'", poLama.NoNota).First(&kasSisa).Error

		if input.IsLunas && sisaTagihan > 0 {
			ketSisa := fmt.Sprintf("Pelunasan Sisa PO - %s (Pemesan: %s)", poLama.NoNota, input.NamaPemesan)
			if errSisa == nil {
				DB.Model(&kasSisa).Updates(map[string]interface{}{"nominal": sisaTagihan, "keterangan": ketSisa})
			} else {
				DB.Create(&models.TransaksiKas{
					Tanggal: time.Now(), Kategori: "PESANAN", Jenis: "MASUK",
					Nominal: sisaTagihan, Keterangan: ketSisa, NoNotaRef: poLama.NoNota, CreatedBy: adminID,
				})
			}
		} else { // Jika Batal Lunas (atau Sisa Tagihan jadi 0), hapus kas pelunasan
			if errSisa == nil {
				DB.Unscoped().Delete(&kasSisa)
			}
		}
	}

	return c.JSON(fiber.Map{"message": "Pesanan diupdate!"})
}

// 1. Get Semua Riwayat Pesanan
func GetRiwayatPesanan(c *fiber.Ctx) error {
	var pesanan []models.NotaPesanan
	// Urutkan dari yang terbaru, hapus Where("riwayat") yang error
	if err := DB.Preload("Toko").Preload("Details").Order("id desc").Find(&pesanan).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(pesanan)
}

// Batalkan Pesanan PO (Soft Cancel & Tarik Kas)
func BatalkanPesanan(c *fiber.Ctx) error {
	id := c.Params("id")
	tx := DB.Begin()

	var pesanan models.NotaPesanan
	if err := tx.First(&pesanan, id).Error; err != nil {
		tx.Rollback()
		return c.Status(404).JSON(fiber.Map{"error": "Pesanan tidak ditemukan"})
	}

	// ==============================================================
	// BARU: REFUND KEMASAN KUSTOM JIKA SUDAH TERPOTONG TUTUP BUKU
	// ==============================================================
	var details []models.NotaPesananDetail
	// Tarik detail pesanan kustom yang gemboknya SUDAH TERKUNCI (true)
	tx.Preload("KemasanDetail").Where("nota_pesanan_id = ? AND is_kemasan_terpotong = ?", id, true).Find(&details)

	for _, pk := range details {
		// Pastikan ini barang kustom (BarangID nil) dan punya kemasan
		if pk.BarangID == nil && len(pk.KemasanDetail) > 0 {
			for _, k := range pk.KemasanDetail {
				totalRefund := float64(pk.Banyak) * k.Kebutuhan
				// Kembalikan (Refund) stok kardus ke master bahan
				tx.Model(&models.Bahan{}).Where("id = ?", k.BahanID).
					Update("stok", gorm.Expr("stok + ?", totalRefund))
			}
		}

		// BUKA GEMBOKNYA: Agar statusnya kembali sinkron
		tx.Model(&models.NotaPesananDetail{}).Where("id = ?", pk.ID).Update("is_kemasan_terpotong", false)
	}
	// ==============================================================

	// 1. Ubah status
	if err := tx.Model(&pesanan).Update("status", "DIBATALKAN").Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 2. Tarik uang DP dan Pelunasan dari Brankas
	tx.Unscoped().Where("no_nota_ref = ?", pesanan.NoNota).Delete(&models.TransaksiKas{})

	tx.Commit()
	return c.JSON(fiber.Map{"message": "Pesanan berhasil dibatalkan dan Kas ditarik kembali!"})
}

// PULIHKAN NOTA PESANAN (PO)
func PulihkanPesanan(c *fiber.Ctx) error {
	id := c.Params("id")
	tx := DB.Begin()

	var pesanan models.NotaPesanan
	if err := tx.First(&pesanan, id).Error; err != nil {
		tx.Rollback()
		return c.Status(404).JSON(fiber.Map{"error": "Pesanan tidak ditemukan"})
	}

	// 1. Kembalikan status
	if err := tx.Model(&pesanan).Update("status", "MENUNGGU").Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 2. Kembalikan Kas (DP & Pelunasan)
	var settingKas models.PengaturanSistem
	tx.Where("key = ?", "ENABLE_KAS_SYNC").First(&settingKas)

	if settingKas.Value == "true" {
		adminID := c.Locals("admin_id").(uint)

		if pesanan.UangMuka > 0 {
			tx.Create(&models.TransaksiKas{
				Tanggal:    time.Now(),
				Kategori:   "PESANAN",
				Jenis:      "MASUK",
				Nominal:    pesanan.UangMuka,
				Keterangan: fmt.Sprintf("DP Pesanan - %s (Pemesan: %s) [DIPULIHKAN]", pesanan.NoNota, pesanan.NamaPemesan),
				NoNotaRef:  pesanan.NoNota,
				CreatedBy:  adminID,
			})
		}

		if pesanan.IsLunas && pesanan.SisaTagihan > 0 {
			tx.Create(&models.TransaksiKas{
				Tanggal:    time.Now(),
				Kategori:   "PESANAN",
				Jenis:      "MASUK",
				Nominal:    pesanan.SisaTagihan,
				Keterangan: fmt.Sprintf("Pelunasan Sisa PO - %s (Pemesan: %s) [DIPULIHKAN]", pesanan.NoNota, pesanan.NamaPemesan),
				NoNotaRef:  pesanan.NoNota,
				CreatedBy:  adminID,
			})
		}
	}

	tx.Commit()
	return c.JSON(fiber.Map{"message": "Pesanan berhasil dipulihkan!"})
}

// 3. Get Rangkuman Khusus Pesanan (Untuk Tab Rangkuman Bulanan)
func GetRangkumanPesanan(c *fiber.Ctx) error {
	start := c.Query("start")
	end := c.Query("end")

	// Total Global
	var summary struct {
		TotalPendapatan float64 `json:"total_pendapatan"`
		TotalPesanan    int     `json:"total_pesanan"`
		TotalDiskon     float64 `json:"total_diskon"`
	}
	// LOGIKA SIMPLE: Semua omzet (dikurangi voucher) diakui penuh di hari H pengiriman
	DB.Model(&models.NotaPesanan{}).
		Where("tanggal_kirim >= ? AND tanggal_kirim <= ? AND status != 'DIBATALKAN'", start, end).
		Select("COALESCE(SUM(total_bayar - total_voucher), 0) as total_pendapatan, COALESCE(SUM(total_voucher), 0) as total_diskon, COUNT(id) as total_pesanan").
		Scan(&summary)

	// Breakdown Per Titik Ambil
	var perTitik []struct {
		NamaTitik  string  `json:"nama_titik"`
		Pendapatan float64 `json:"pendapatan"`
		Diskon     float64 `json:"diskon"`
		TotalNota  int     `json:"total_nota"`
	}
	DB.Model(&models.NotaPesanan{}).
		Where("tanggal_kirim >= ? AND tanggal_kirim <= ? AND status != 'DIBATALKAN'", start, end).
		Select("nama_toko_snapshot as nama_titik, COALESCE(SUM(total_bayar - total_voucher), 0) as pendapatan, COALESCE(SUM(total_voucher), 0) as diskon, COUNT(id) as total_nota").
		Group("nama_toko_snapshot").
		Order("pendapatan desc").
		Scan(&perTitik)

	// Detail Pesanan per Barang
	var detailBarang []struct {
		NamaTitik   string  `json:"nama_titik"`
		NamaBarang  string  `json:"nama_barang"`
		TotalQty    int     `json:"total_qty"`
		TotalRupiah float64 `json:"total_rupiah"`
	}
	DB.Table("nota_pesanan_details").
		Select("nota_pesanans.nama_toko_snapshot as nama_titik, nota_pesanan_details.nama_barang_bebas as nama_barang, SUM(nota_pesanan_details.banyak) as total_qty, SUM(nota_pesanan_details.subtotal) as total_rupiah").
		Joins("join nota_pesanans on nota_pesanans.id = nota_pesanan_details.nota_pesanan_id").
		Where("nota_pesanans.tanggal_kirim >= ? AND nota_pesanans.tanggal_kirim <= ? AND nota_pesanans.status != 'DIBATALKAN'", start, end).
		Group("nota_pesanans.nama_toko_snapshot, nota_pesanan_details.nama_barang_bebas").
		Order("nama_titik asc, total_qty desc").
		Scan(&detailBarang)

	// Kembalikan datanya ke Vue
	return c.JSON(fiber.Map{
		"total_pendapatan": summary.TotalPendapatan,
		"total_pesanan":    summary.TotalPesanan,
		"total_diskon":     summary.TotalDiskon, // Murni Voucher saja
		"per_titik":        perTitik,
		"detail_barang":    detailBarang,
	})
}

// DASHBOARD KUNJUNGAN SALES
func GetDashboardSales(c *fiber.Ctx) error {
	adminID := c.Locals("admin_id").(uint)
	var notaAktif []models.Nota
	var notaTugas []models.Nota
	var poTugas []models.NotaPesanan

	// Nota Aktif: 8 jam terakhir, status bebas
	DB.Preload("Toko").Where("created_by = ? AND created_at >= ?", adminID, time.Now().Add(-8*time.Hour)).Order("id desc").Find(&notaAktif)

	// Tugas Khusus (Reguler) dari Superadmin
	DB.Preload("Toko").Where("assigned_to = ? AND (jumlah_retur = 0 OR updated_at > ?)", adminID, time.Now().Add(-12*time.Hour)).Order("id desc").Find(&notaTugas)

	// BARU: Tugas Khusus Pesanan (PO) dari Superadmin yang BELUM SELESAI
	DB.Where("assigned_to = ? AND status != 'DIAMBIL'", adminID).Order("id desc").Find(&poTugas)

	// Kirim semua tugas ke Vue
	return c.JSON(fiber.Map{"aktif": notaAktif, "tugas": notaTugas, "tugas_po": poTugas})
}

func GetKunjunganToko(c *fiber.Ctx) error { // Memeriksa tagihan Retur saat tiba di toko
	tokoID := c.Params("toko_id")
	var notaBelumRetur []models.Nota

	DB.Preload("Toko").Where("toko_id = ? AND status = 'KIRIM' AND jumlah_retur = 0 AND tanggal_kirim >= ?",
		tokoID, time.Now().AddDate(0, -1, 0)).Order("tanggal_kirim asc").Find(&notaBelumRetur)

	return c.JSON(notaBelumRetur)
}

// 1. Tarik Data Kas (Bisa difilter per bulan/kategori nanti di Vue)
func GetKas(c *fiber.Ctx) error {
	var kas []models.TransaksiKas
	// Urutkan dari yang terbaru (tanggal terbaru, inputan terakhir)
	if err := DB.Order("tanggal desc, id desc").Find(&kas).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(kas)
}

// 2. Input Kas Manual (Untuk Kategori RUMAH_TANGGA atau Setoran)
func CreateKas(c *fiber.Ctx) error {
	var input struct {
		Tanggal    string  `json:"tanggal"`
		Kategori   string  `json:"kategori"` // REGULER, PESANAN, BAHAN, RUMAH_TANGGA
		Jenis      string  `json:"jenis"`    // MASUK, KELUAR
		Nominal    float64 `json:"nominal"`
		Keterangan string  `json:"keterangan"`
		NoNotaRef  string  `json:"no_nota_ref"`
	}

	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format data salah"})
	}

	tgl, _ := time.Parse("2006-01-02", input.Tanggal)
	adminID := c.Locals("admin_id").(uint) // Ambil ID pembuat dari token JWT

	kas := models.TransaksiKas{
		Tanggal:    tgl,
		Kategori:   input.Kategori,
		Jenis:      input.Jenis,
		Nominal:    input.Nominal,
		Keterangan: input.Keterangan,
		NoNotaRef:  input.NoNotaRef,
		CreatedBy:  adminID,
	}

	if err := DB.Create(&kas).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Transaksi kas berhasil dicatat!", "id": kas.ID})
}

// 3. Hapus Kas (Kalau salah ketik / salah input)
func DeleteKas(c *fiber.Ctx) error {
	id := c.Params("id")
	if err := DB.Delete(&models.TransaksiKas{}, id).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Transaksi kas berhasil dihapus!"})
}

// PENGATURAN SAKLAR KAS
func GetPengaturanKas(c *fiber.Ctx) error {
	var setting models.PengaturanSistem
	DB.Where("key = ?", "ENABLE_KAS_SYNC").First(&setting)

	isActive := setting.Value == "true"
	return c.JSON(fiber.Map{"is_active": isActive})
}

func TogglePengaturanKas(c *fiber.Ctx) error {
	var input struct {
		IsActive bool `json:"is_active"`
	}

	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format data salah"})
	}

	newVal := "false"
	if input.IsActive {
		newVal = "true"
	}

	// Update ke Database
	DB.Model(&models.PengaturanSistem{}).Where("key = ?", "ENABLE_KAS_SYNC").Update("value", newVal)

	return c.JSON(fiber.Map{
		"message":   "Status sinkronisasi kas diperbarui!",
		"is_active": input.IsActive,
	})
}

// HANDLER ANALISIS ASET & PERTUMBUHAN
func GetAnalisisAsetLive(c *fiber.Ctx) error {
	// Ambil parameter tanggal dari URL, defaultnya hari ini
	targetDate := c.Query("date")
	startDatePrive := c.Query("start_date") // Tanggal Mulai Hitung Prive

	if targetDate == "" {
		targetDate = time.Now().Format("2006-01-02")
	}

	tglTarget, _ := time.Parse("2006-01-02", targetDate)

	// Jika start_date kosong, default ke tanggal 1 di bulan yang sama dengan targetDate
	if startDatePrive == "" {
		startDatePrive = time.Date(tglTarget.Year(), tglTarget.Month(), 1, 0, 0, 0, 0, time.Local).Format("2006-01-02")
	}

	// 1. TOTAL KAS (Sampai tanggal target)
	var totalMasuk, totalKeluar float64
	DB.Model(&models.TransaksiKas{}).Where("jenis = 'MASUK' AND tanggal <= ?", targetDate).Select("COALESCE(SUM(nominal), 0)").Row().Scan(&totalMasuk)
	DB.Model(&models.TransaksiKas{}).Where("jenis = 'KELUAR' AND tanggal <= ?", targetDate).Select("COALESCE(SUM(nominal), 0)").Row().Scan(&totalKeluar)
	kasLive := totalMasuk - totalKeluar

	// 2. TOTAL PIUTANG (Masih hutang dan dibuat sebelum/pada tanggal target)
	var piutangReguler, piutangPO float64
	DB.Model(&models.Nota{}).Where("is_lunas = false AND tanggal_kirim <= ?", targetDate).Select("COALESCE(SUM(total_bayar), 0)").Row().Scan(&piutangReguler)
	DB.Model(&models.NotaPesanan{}).Where("is_lunas = false AND status != 'DIBATALKAN' AND tanggal_kirim <= ?", targetDate).Select("COALESCE(SUM(sisa_tagihan), 0)").Row().Scan(&piutangPO)
	piutangLive := piutangReguler + piutangPO

	// 3. TOTAL HUTANG (Belum lunas dan dibeli sebelum/pada tanggal target)
	var hutangLive float64
	DB.Model(&models.PembelianBahan{}).Where("is_lunas = false AND tanggal <= ?", targetDate).Select("COALESCE(SUM(total_biaya), 0)").Row().Scan(&hutangLive)

	// 4. NILAI PERSEDIAAN (Menggunakan stok saat ini)
	var inventoryLive float64
	DB.Model(&models.Bahan{}).Select("COALESCE(SUM(stok * harga_saat_ini), 0)").Row().Scan(&inventoryLive)

	// 5. HITUNG PRIVE (Flow: Dari start_date sampai targetDate)
	var totalPrive float64
	DB.Model(&models.TransaksiKas{}).
		Where("kategori = 'RUMAH_TANGGA' AND jenis = 'KELUAR' AND tanggal >= ? AND tanggal <= ?", startDatePrive, targetDate).
		Select("COALESCE(SUM(nominal), 0)").Row().Scan(&totalPrive)

	// 6. AMBIL DATA BULAN LALU (Snapshot terakhir sebelum targetDate)
	var snapshotLalu models.AsetSnapshot
	DB.Where("bulan < ?", targetDate).Order("bulan desc").First(&snapshotLalu)

	return c.JSON(fiber.Map{
		"live": fiber.Map{
			"total_kas":        kasLive, // Hasil (Masuk - Keluar) s/d targetDate
			"total_piutang":    piutangLive,
			"total_persediaan": inventoryLive,
			"total_hutang":     hutangLive,
			"aset_bersih":      kasLive + piutangLive + inventoryLive - hutangLive,
		},
		"prive_bulan_ini":  totalPrive,
		"bulan_lalu":       snapshotLalu,
		"tanggal_analisis": targetDate,
		"awal_prive":       startDatePrive,
	})
}

// FUNGSI UNTUK MENGUNCI (SNAPSHOT) ASET AKHIR BULAN
func SimpanSnapshotAset(c *fiber.Ctx) error {
	var input struct {
		Bulan string `json:"bulan"` // Format: 2026-05-01
	}
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format tanggal salah"})
	}

	tglStr := input.Bulan
	tgl, _ := time.Parse("2006-01-02", tglStr)

	var kM, kK, pR, pPO, inv, hL float64

	// 1. KAS (Masuk dan Keluar DIBATASI sampai tanggal snapshot)
	DB.Model(&models.TransaksiKas{}).Where("jenis = 'MASUK' AND tanggal <= ?", tglStr).Select("COALESCE(SUM(nominal), 0)").Row().Scan(&kM)
	DB.Model(&models.TransaksiKas{}).Where("jenis = 'KELUAR' AND tanggal <= ?", tglStr).Select("COALESCE(SUM(nominal), 0)").Row().Scan(&kK)

	// 2. PIUTANG (Nota/PO yang dibuat s/d tanggal snapshot dan belum lunas)
	DB.Model(&models.Nota{}).Where("is_lunas = false AND tanggal_kirim <= ?", tglStr).Select("COALESCE(SUM(total_bayar), 0)").Row().Scan(&pR)
	DB.Model(&models.NotaPesanan{}).Where("is_lunas = false AND status != 'DIBATALKAN' AND tanggal_kirim <= ?", tglStr).Select("COALESCE(SUM(sisa_tagihan), 0)").Row().Scan(&pPO)

	// 3. HUTANG (Pembelian s/d tanggal snapshot yang belum lunas)
	DB.Model(&models.PembelianBahan{}).Where("is_lunas = false AND tanggal <= ?", tglStr).Select("COALESCE(SUM(total_biaya), 0)").Row().Scan(&hL)

	// 4. PERSEDIAAN (Khusus stok gudang selalu mengambil keadaan real-time saat tombol dikunci)
	DB.Model(&models.Bahan{}).Select("COALESCE(SUM(stok * harga_saat_ini), 0)").Row().Scan(&inv)

	kas := kM - kK
	piutang := pR + pPO
	bersih := kas + piutang + inv - hL

	snapshot := models.AsetSnapshot{
		Bulan:           tgl,
		TotalKas:        kas,
		TotalPiutang:    piutang,
		TotalPersediaan: inv,
		TotalHutang:     hL,
		AsetBersih:      bersih,
	}

	// Simpan atau Update (Timpa jika bulan tersebut sudah ada)
	if err := DB.Where("bulan = ?", tglStr).First(&models.AsetSnapshot{}).Error; err == nil {
		DB.Model(&models.AsetSnapshot{}).Where("bulan = ?", tglStr).Updates(snapshot)
	} else {
		DB.Create(&snapshot)
	}

	return c.JSON(fiber.Map{"message": "Snapshot aset berhasil dikunci!"})
}

func GetRiwayatAset(c *fiber.Ctx) error {
	var riwayat []models.AsetSnapshot
	DB.Order("bulan desc").Find(&riwayat)
	return c.JSON(riwayat)
}

// MAIN
func main() {
	connectDB()

	app := fiber.New()

	app.Use(cors.New(cors.Config{
		AllowOrigins: "http://localhost:5173, http://localhost:5174, http://localhost:5175, https://nota-tiara-frontend.vercel.app, https://tiara-inventory.vercel.app, https://kas-tiara.vercel.app",
	}))

	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString("Backend Tiara Connected with Env!")
	})

	app.Post("/login", LoginAdmin) // Endpoint Publik (Bisa diakses tanpa token)

	// Kelompokkan rute yang butuh login
	api := app.Group("/api", Protected)

	api.Get("/admins", RequireRole(RoleSuperadmin, RoleSales), func(c *fiber.Ctx) error {
		var admins []models.Admin
		// Ambil semua admin, lalu kirim ke Vue
		if err := DB.Find(&admins).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(admins)
	})

	// BAHAN
	api.Get("/bahan", RequireRole(RoleSuperadmin), GetBahan)
	api.Put("/bahan/reorder", RequireRole(RoleSuperadmin), UpdateUrutanBahan)
	api.Post("/bahan", RequireRole(RoleSuperadmin), CreateBahan)
	api.Put("/bahan/:id", RequireRole(RoleSuperadmin), UpdateBahan)
	api.Delete("/bahan/:id", RequireRole(RoleSuperadmin), DeleteBahan)

	// PEMBELIAN
	api.Get("/pembelian", RequireRole(RoleSuperadmin), GetPembelianBahan)
	api.Post("/pembelian", RequireRole(RoleSuperadmin), CreatePembelianBahan)
	api.Put("/pembelian/:id/status", RequireRole(RoleSuperadmin), UpdateStatusPembelian)
	api.Delete("/pembelian/:id", RequireRole(RoleSuperadmin), DeletePembelianBahan)

	// RESEP
	api.Get("/resep", RequireRole(RoleSuperadmin), GetResep)
	api.Post("/resep", RequireRole(RoleSuperadmin), CreateResep)
	api.Put("/resep/:id", RequireRole(RoleSuperadmin), UpdateResep)
	api.Delete("/resep/:id", RequireRole(RoleSuperadmin), DeleteResep)

	// PRODUKSI HARIAN
	api.Get("/produksi/masak", RequireRole(RoleSuperadmin), GetProduksiMasak)
	api.Post("/produksi/masak", RequireRole(RoleSuperadmin), CreateProduksiMasak)
	api.Delete("/produksi/masak/:id", RequireRole(RoleSuperadmin), DeleteProduksiMasak)
	api.Get("/produksi/matang", RequireRole(RoleSuperadmin), GetProduksiMatang)
	api.Post("/produksi/matang", RequireRole(RoleSuperadmin), CreateProduksiMatang)
	api.Delete("/produksi/matang/:id", RequireRole(RoleSuperadmin), DeleteProduksiMatang)

	// TUTUP BUKU & OPNAME
	api.Post("/produksi/tutup-buku", RequireRole(RoleSuperadmin), TutupBukuHarian)
	api.Get("/produksi/jurnal", RequireRole(RoleSuperadmin), GetJurnalTutupBuku)

	// AFKIR / BARANG RUSAK
	api.Get("/inventory/rusak", RequireRole(RoleSuperadmin), GetBarangRusak)
	api.Post("/inventory/rusak", RequireRole(RoleSuperadmin), CreateBarangRusak)
	api.Delete("/inventory/rusak/:id", RequireRole(RoleSuperadmin), DeleteBarangRusak)

	// TUTUP BUKU & OPNAME
	api.Post("/produksi/tutup-buku", RequireRole(RoleSuperadmin), TutupBukuHarian)
	api.Get("/produksi/jurnal", RequireRole(RoleSuperadmin), GetJurnalTutupBuku)
	api.Get("/opname", RequireRole(RoleSuperadmin), GetOpname)
	api.Post("/opname", RequireRole(RoleSuperadmin), CreateOpname)
	api.Get("/konversi/sisa-kemarin", RequireRole(RoleSuperadmin), GetSisaLayakJualKemarin)

	// BARANG
	api.Get("/barangs", RequireRole(RoleSuperadmin, RoleSales), GetBarangs)
	api.Put("/barangs/reorder", RequireRole(RoleSuperadmin), UpdateUrutanBarang)
	api.Post("/barangs", RequireRole(RoleSuperadmin), CreateBarang)
	api.Put("/barangs/:id", RequireRole(RoleSuperadmin), UpdateBarang)
	api.Delete("/barangs/:id", RequireRole(RoleSuperadmin), DeleteBarang)

	// TOKO
	api.Get("/tokos", RequireRole(RoleSuperadmin, RoleSales), GetTokos)
	api.Post("/tokos", RequireRole(RoleSuperadmin), CreateToko)
	api.Put("/tokos/:id", RequireRole(RoleSuperadmin), UpdateToko)
	api.Delete("/tokos/:id", RequireRole(RoleSuperadmin), DeleteToko)

	// NOTA
	api.Get("/profil", RequireRole(RoleSuperadmin, RoleSales), GetProfilTiara)
	api.Get("/notas/next-number", RequireRole(RoleSuperadmin, RoleSales), GetNextNotaNumber)
	api.Get("/notas", RequireRole(RoleSuperadmin), GetNotas)
	api.Get("/notas/:id", RequireRole(RoleSuperadmin, RoleSales), GetNotaByID)
	api.Post("/notas", RequireRole(RoleSuperadmin, RoleSales), CreateNota)
	api.Put("/notas/:id", RequireRole(RoleSuperadmin, RoleSales), UpdateNota)
	api.Put("/notas/:id/batal", RequireRole(RoleSuperadmin, RoleSales), BatalkanNota)
	api.Put("/notas/:id/pulihkan", RequireRole(RoleSuperadmin, RoleSales), PulihkanNota)

	// CATATAN BESAR
	api.Get("/catatan-besar", RequireRole(RoleSuperadmin), GetCatatanBesar)

	// RANGKUMAN
	api.Get("/rangkuman", RequireRole(RoleSuperadmin), GetRangkuman)
	api.Get("/rangkuman-per-toko", RequireRole(RoleSuperadmin), GetRangkumanPerToko)

	// KAS
	api.Get("/kas", RequireRole(RoleSuperadmin), GetKas)
	api.Post("/kas", RequireRole(RoleSuperadmin), CreateKas)
	api.Delete("/kas/:id", RequireRole(RoleSuperadmin), DeleteKas)

	// SAKLAR KAS (TAMBAHKAN INI)
	api.Get("/pengaturan/kas", RequireRole(RoleSuperadmin), GetPengaturanKas)
	api.Put("/pengaturan/kas", RequireRole(RoleSuperadmin), TogglePengaturanKas)

	// SAMPAH
	api.Get("/sampah", RequireRole(RoleSuperadmin), GetTrash)
	api.Put("/sampah/:type/:id", RequireRole(RoleSuperadmin), RestoreData)

	// NOTA PESANAN (RUTE STATIS DI ATAS)
	api.Get("/pesanan/next-number", RequireRole(RoleSuperadmin), GetNextNotaPesananNumber)
	api.Get("/pesanan/catatan", RequireRole(RoleSuperadmin), GetCatatanPesanan)
	api.Get("/pesanan/riwayat", RequireRole(RoleSuperadmin), GetRiwayatPesanan)
	api.Get("/pesanan/rangkuman-bulanan", RequireRole(RoleSuperadmin), GetRangkumanPesanan)

	// NOTA PESANAN (RUTE DINAMIS DENGAN :id WAJIB DI BAWAH)
	api.Post("/pesanan", RequireRole(RoleSuperadmin), CreateNotaPesanan)
	api.Get("/pesanan/:id", RequireRole(RoleSuperadmin, RoleSales), GetNotaPesananByID)
	api.Post("/pesanan/:id", RequireRole(RoleSuperadmin), UpdateNotaPesanan)
	api.Put("/pesanan/:id/batal", RequireRole(RoleSuperadmin), BatalkanPesanan)
	api.Put("/pesanan/:id/pulihkan", RequireRole(RoleSuperadmin), PulihkanPesanan)

	// DASHBOARD KUNJUNGAN SALES
	api.Get("/sales/dashboard", RequireRole(RoleSuperadmin, RoleSales), GetDashboardSales)
	api.Get("/sales/kunjungan/:toko_id", RequireRole(RoleSuperadmin, RoleSales), GetKunjunganToko)

	// ANALISIS ASET & PERTUMBUHAN
	api.Get("/aset/live", RequireRole(RoleSuperadmin), GetAnalisisAsetLive)
	api.Post("/aset/snapshot", RequireRole(RoleSuperadmin), SimpanSnapshotAset)
	api.Get("/aset/riwayat", RequireRole(RoleSuperadmin), GetRiwayatAset)

	appPort := os.Getenv("PORT")
	if appPort == "" {
		appPort = "3000"
	}

	log.Println("Server jalan di port " + appPort)
	log.Fatal(app.Listen(":" + appPort))
}
