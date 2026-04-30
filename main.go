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
	)
	log.Println("Database & Tabel Berhasil Disiapkan! 🏗️")

	// // 1. Cek & Buat Super Admin
	// var adminAccount models.Admin
	// if err := DB.Where("username = ?", "admin").First(&adminAccount).Error; err != nil {
	// 	// Jika tidak ditemukan, baru buat
	// 	hashedAdmin, _ := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
	// 	DB.Create(&models.Admin{
	// 		Username: "admin",
	// 		Password: string(hashedAdmin),
	// 		Role:     "superadmin",
	// 	})
	// 	log.Println("✅ Akun Super Admin 'admin' siap!")
	// }

	// // 2. Cek & Buat Akun Sales
	// var salesAccount models.Admin
	// if err := DB.Where("username = ?", "sales1").First(&salesAccount).Error; err != nil {
	// 	// Jika sales1 belum ada, buatkan otomatis
	// 	hashedSales, _ := bcrypt.GenerateFromPassword([]byte("sales123"), bcrypt.DefaultCost)
	// 	DB.Create(&models.Admin{
	// 		Username: "sales1",
	// 		Password: string(hashedSales),
	// 		Role:     "sales", // <--- UBAH JADI sales
	// 	})
	// 	log.Println("✅ Akun Sales 'sales1' siap!")
	// }

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

// BUAT NOTA
func GetNextNotaNumber(c *fiber.Ctx) error {
	tokoID := c.Query("toko_id")
	tgl := c.Query("tanggal") // Format: 2026-04-27

	// Hilangkan tanda strip pada tanggal: 2026-04-27 -> 20260427
	tglStr := strings.ReplaceAll(tgl, "-", "")

	var count int64
	// Gunakan Unscoped agar nota yang dibatalkan/dihapus tetap terhitung
	// sehingga nomornya tidak akan mundur / dipakai ulang
	DB.Unscoped().Model(&models.Nota{}).Where("toko_id = ?", tokoID).Count(&count)

	nextUrutan := count + 1

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
	return c.JSON(fiber.Map{"message": "Nota berhasil dibuat!", "id": nota.ID})
}

func UpdateNota(c *fiber.Ctx) error {
	id := c.Params("id")
	var input struct {
		AssignedTo uint   `json:"assigned_to"`
		Status     string `json:"status"`
		IsLunas    bool   `json:"is_lunas"`
		Details    []struct {
			ID          uint    `json:"id"`
			BarangID    uint    `json:"barang_id"`
			BanyakRetur int     `json:"banyak_retur"`
			HargaJual   float64 `json:"harga_jual"`
		} `json:"details"`
	}

	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	for _, d := range input.Details {
		hRetur := float64(d.BanyakRetur) * d.HargaJual

		if d.ID != 0 {
			// Kasus 1: Detail sudah ada, cukup update
			DB.Model(&models.NotaDetail{}).Where("id = ?", d.ID).Updates(map[string]interface{}{
				"banyak_retur": d.BanyakRetur,
				"harga_retur":  hRetur,
			})
		} else if d.BanyakRetur > 0 {
			// Kasus 2: Detail belum ada (Toko Harian retur barang yang tidak dikirim hari itu)
			// harus buat baris baru di nota_details
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
				BanyakKirim:        0,
				HargaJual:          d.HargaJual,
				HargaKirim:         0,
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

	DB.Model(&models.Nota{}).Where("id = ?", id).Updates(map[string]interface{}{
		"jumlah_retur": totalRetur,
		"total_bayar":  totalKirim - totalRetur,
		"assigned_to":  input.AssignedTo,
		"status":       input.Status,
		"is_lunas":     input.IsLunas,
	})

	return c.JSON(fiber.Map{"message": "Nota berhasil diupdate!"})
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
	var results []struct {
		NamaBarang string  `json:"nama_barang"`
		NamaToko   string  `json:"nama_toko"`
		QtyKirim   int     `json:"qty_kirim"`
		QtyRetur   int     `json:"qty_retur"`
		HargaKirim float64 `json:"harga_kirim"`
	}

	err := DB.Table("nota_details").
		Select("barangs.nama_barang, tokos.nama_toko, SUM(nota_details.banyak_kirim) as qty_kirim, SUM(nota_details.banyak_retur) as qty_retur, SUM(nota_details.harga_kirim) as harga_kirim").
		Joins("join notas on notas.id = nota_details.nota_id").
		Joins("join tokos on tokos.id = notas.toko_id").
		Joins("join barangs on barangs.id = nota_details.barang_id").
		Where("tokos."+siklus+" = ?", true).
		Where("notas.tanggal_kirim >= ?", time.Now().AddDate(0, 0, -7)).
		Group("barangs.nama_barang, tokos.nama_toko").
		Scan(&results).Error

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(results)
}

// MASTER BARANG
func GetBarangs(c *fiber.Ctx) error {
	var barangs []models.Barang
	if err := DB.Order("urutan asc, id asc").Find(&barangs).Error; err != nil {
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
	var input models.Barang
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	// Cari nilai "urutan" tertinggi di database saat ini
	var maxUrutan int
	// Gunakan COALESCE agar kalau databasenya kosong, dia mulai dari 0
	DB.Model(&models.Barang{}).Select("COALESCE(MAX(urutan), 0)").Row().Scan(&maxUrutan)

	// Set urutan barang baru agar otomatis jatuh di paling bawah (+1 dari yang tertinggi)
	input.Urutan = maxUrutan + 1

	// Simpan ke database
	if err := DB.Create(&input).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(input)
}

func UpdateBarang(c *fiber.Ctx) error {
	id := c.Params("id")
	var input models.Barang

	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	var barang models.Barang
	// Cek apakah barangnya ada
	if err := DB.First(&barang, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Barang tidak ditemukan"})
	}

	// Update data barang
	DB.Model(&barang).Updates(input)
	return c.JSON(fiber.Map{"message": "Barang berhasil diupdate", "data": barang})
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
		rekapMap[t.ID] = &models.RekapToko{ID: t.ID, Nama: t.NamaToko, Kirim: 0, Retur: 0, Pendapatan: 0, Persentase: 0}
	}

	// =========================================================================
	// RUMUS PINTAR KALENDER JANGKAR TIARA (FIXED ANCHOR DAYS)
	// Menggunakan DATE_TRUNC('week') untuk mengunci hari Senin di minggu itu,
	// lalu ditambah hari statis, tidak peduli hari apa nota itu diinput.
	// =========================================================================
	kirimDateExpr := `CAST(
		CASE 
			WHEN nota.siklus_snapshot = 'HARIAN' THEN nota.tanggal_kirim
			WHEN nota.siklus_snapshot = 'SiklusKamisSenin' THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '3 days'
			WHEN nota.siklus_snapshot = 'SiklusJumatSelasa' THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '4 days'
			WHEN nota.siklus_snapshot = 'SiklusSabtuRabu' THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '5 days'
			ELSE nota.tanggal_kirim
		END AS DATE)`

	returDateExpr := `CAST(
		CASE 
			WHEN nota.siklus_snapshot = 'HARIAN' THEN nota.tanggal_kirim
			WHEN nota.siklus_snapshot = 'SiklusKamisSenin' THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '7 days'
			WHEN nota.siklus_snapshot = 'SiklusJumatSelasa' THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '8 days'
			WHEN nota.siklus_snapshot = 'SiklusSabtuRabu' THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '9 days'
			ELSE nota.tanggal_kirim + INTERVAL '4 days'
		END AS DATE)`

	var rawResults []struct {
		ID    uint
		Nama  string
		Kirim float64
		Retur float64
	}

	// 3. EKSEKUSI SQL TOKO
	queryToko := fmt.Sprintf(`
		SELECT 
			toko_id as id,
			MAX(nama_toko_snapshot) as nama,
			COALESCE(SUM(CASE WHEN %s >= CAST(? AS DATE) AND %s <= CAST(? AS DATE) THEN jumlah_kirim ELSE 0 END), 0) as kirim,
			COALESCE(SUM(CASE WHEN %s >= CAST(? AS DATE) AND %s <= CAST(? AS DATE) THEN jumlah_retur ELSE 0 END), 0) as retur
		FROM nota
		WHERE 
			(%s >= CAST(? AS DATE) AND %s <= CAST(? AS DATE))
			OR 
			(%s >= CAST(? AS DATE) AND %s <= CAST(? AS DATE))
		GROUP BY toko_id
	`, kirimDateExpr, kirimDateExpr, returDateExpr, returDateExpr, kirimDateExpr, kirimDateExpr, returDateExpr, returDateExpr)

	// Butuh 8 parameter: 4 untuk cek SELECT, 4 untuk cek WHERE
	DB.Raw(queryToko, startDate, endDate, startDate, endDate, startDate, endDate, startDate, endDate).Scan(&rawResults)

	var totalKirim, totalRetur float64

	for _, r := range rawResults {
		if val, exists := rekapMap[r.ID]; exists {
			val.Kirim = r.Kirim
			val.Retur = r.Retur
			val.Pendapatan = r.Kirim - r.Retur
			if r.Kirim > 0 {
				val.Persentase = (r.Retur / r.Kirim) * 100
			}
			if r.Nama != "" {
				val.Nama = r.Nama
			}
		}
		totalKirim += r.Kirim
		totalRetur += r.Retur
	}

	var perToko []models.RekapToko
	for _, r := range rekapMap {
		perToko = append(perToko, *r)
	}

	sort.Slice(perToko, func(i, j int) bool { return perToko[i].Pendapatan > perToko[j].Pendapatan })

	totalPersentase := 0.0
	if totalKirim > 0 {
		totalPersentase = (totalRetur / totalKirim) * 100
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
			(%s >= CAST(? AS DATE) AND %s <= CAST(? AS DATE))
			OR 
			(%s >= CAST(? AS DATE) AND %s <= CAST(? AS DATE))
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
		Pendapatan: totalKirim - totalRetur,
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
		NamaBarang string `json:"nama_barang"`
		TotalKirim int    `json:"total_kirim"`
		TotalRetur int    `json:"total_retur"`
		TotalLaku  int    `json:"total_laku"`
	}

	kirimDateExpr := `CAST(
		CASE 
			WHEN nota.siklus_snapshot = 'HARIAN' THEN nota.tanggal_kirim
			WHEN nota.siklus_snapshot = 'SiklusKamisSenin' THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '3 days'
			WHEN nota.siklus_snapshot = 'SiklusJumatSelasa' THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '4 days'
			WHEN nota.siklus_snapshot = 'SiklusSabtuRabu' THEN DATE_TRUNC('week', nota.tanggal_kirim) + INTERVAL '5 days'
			ELSE nota.tanggal_kirim
		END AS DATE)`

	returDateExpr := `CAST(
		CASE 
			WHEN nota.siklus_snapshot = 'HARIAN' THEN nota.tanggal_kirim
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
		GROUP BY nota_details.barang_id
	`, kirimDateExpr, kirimDateExpr, returDateExpr, returDateExpr, kirimDateExpr, kirimDateExpr, returDateExpr, returDateExpr)

	// Parameter dinamis harus pas 9 buah (8 dari tanggal, 1 tokoID)
	DB.Raw(query, start, end, start, end, start, end, start, end, tokoID).Scan(&hasil)

	for i := range hasil {
		hasil[i].TotalLaku = hasil[i].TotalKirim - hasil[i].TotalRetur
	}

	sort.Slice(hasil, func(i, j int) bool { return hasil[i].TotalLaku > hasil[j].TotalLaku })

	return c.JSON(hasil)
}

// SAMPAH
func GetTrash(c *fiber.Ctx) error {
	var tokoTerhapus []models.Toko
	var barangTerhapus []models.Barang

	DB.Unscoped().Where("deleted_at IS NOT NULL").Find(&tokoTerhapus)
	DB.Unscoped().Where("deleted_at IS NOT NULL").Find(&barangTerhapus)

	return c.JSON(fiber.Map{
		"tokos":   tokoTerhapus,
		"barangs": barangTerhapus,
	})
}

func RestoreData(c *fiber.Ctx) error {
	jenis := c.Params("type") // "toko" atau "barang"
	id := c.Params("id")

	if jenis == "toko" {
		DB.Unscoped().Model(&models.Toko{}).Where("id = ?", id).Update("deleted_at", nil)
	} else {
		DB.Unscoped().Model(&models.Barang{}).Where("id = ?", id).Update("deleted_at", nil)
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

	var count int64
	// Hitung nota pada hari itu khusus untuk toko tersebut
	if tokoID == "0" {
		DB.Unscoped().Model(&models.NotaPesanan{}).Where("toko_id IS NULL AND DATE(tanggal_kirim) = ?", tgl).Count(&count)
	} else {
		DB.Unscoped().Model(&models.NotaPesanan{}).Where("toko_id = ? AND DATE(tanggal_kirim) = ?", tokoID, tgl).Count(&count)
	}

	nextUrutan := count + 1
	// Format: PO/20260430/0-0001 (Pabrik) atau PO/20260430/15-0001 (Mitra)
	noNota := fmt.Sprintf("PO/%s/%s-%04d", tglStr, tokoID, nextUrutan)

	return c.JSON(fiber.Map{"no_nota": noNota})
}

func CreateNotaPesanan(c *fiber.Ctx) error {
	var input struct {
		NoNota           string `json:"no_nota"`
		NamaPemesan      string `json:"nama_pemesan"`
		TanggalKirim     string `json:"tanggal_kirim"`
		JenisPengambilan string `json:"jenis_pengambilan"`
		TokoID           *uint  `json:"toko_id"`
		AssignedTo       uint   `json:"assigned_to"`
		Status           string `json:"status"`
		IsLunas          bool   `json:"is_lunas"`
		Details          []struct {
			BarangID        *uint   `json:"barang_id"`
			NamaBarangBebas string  `json:"nama_barang_bebas"`
			Banyak          int     `json:"banyak"`
			HargaJual       float64 `json:"harga_jual"`
			ResepID         *uint   `json:"resep_id"` // <--- TAMBAHAN: Tangkap resep
			Gramasi         float64 `json:"gramasi"`  // <--- TAMBAHAN: Tangkap gramasi
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

		pesanan.Details = append(pesanan.Details, models.NotaPesananDetail{
			BarangID:        d.BarangID,
			NamaBarangBebas: d.NamaBarangBebas,
			Banyak:          d.Banyak,
			HargaJual:       d.HargaJual,
			Subtotal:        subtotal,
			ResepID:         d.ResepID, // <--- TAMBAHAN: Simpan resep
			Gramasi:         d.Gramasi, // <--- TAMBAHAN: Simpan gramasi
		})
	}
	pesanan.TotalBayar = totalBayar

	if err := DB.Create(&pesanan).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
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
	if err := DB.Preload("Toko").Preload("Details").Preload("Details.Barang").First(&pesanan, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Pesanan tidak ditemukan"})
	}
	return c.JSON(pesanan)
}

// UPDATE PO
func UpdateNotaPesanan(c *fiber.Ctx) error {
	id := c.Params("id")
	var input struct {
		NamaPemesan      string `json:"nama_pemesan"`
		TanggalKirim     string `json:"tanggal_kirim"`
		JenisPengambilan string `json:"jenis_pengambilan"`
		TokoID           *uint  `json:"toko_id"`
		AssignedTo       uint   `json:"assigned_to"`
		Status           string `json:"status"`
		IsLunas          bool   `json:"is_lunas"`
		Details          []struct {
			BarangID        *uint   `json:"barang_id"`
			NamaBarangBebas string  `json:"nama_barang_bebas"`
			Banyak          int     `json:"banyak"`
			HargaJual       float64 `json:"harga_jual"`
			ResepID         *uint   `json:"resep_id"`
			Gramasi         float64 `json:"gramasi"`
		} `json:"details"`
	}

	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format data salah"})
	}

	tgl, _ := time.Parse("2006-01-02", input.TanggalKirim)

	// Bersihkan detail lama (replace total)
	DB.Where("nota_pesanan_id = ?", id).Delete(&models.NotaPesananDetail{})

	var totalBayar float64
	var newDetails []models.NotaPesananDetail

	for _, d := range input.Details {
		sub := float64(d.Banyak) * d.HargaJual
		totalBayar += sub
		parsedID, _ := strconv.Atoi(id)

		newDetails = append(newDetails, models.NotaPesananDetail{
			NotaPesananID:   uint(parsedID),
			BarangID:        d.BarangID,
			NamaBarangBebas: d.NamaBarangBebas,
			Banyak:          d.Banyak,
			HargaJual:       d.HargaJual,
			Subtotal:        sub,
			ResepID:         d.ResepID,
			Gramasi:         d.Gramasi,
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
	})

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

// 2. Batalkan Pesanan (Soft Cancel, ubah status)
func BatalkanPesanan(c *fiber.Ctx) error {
	id := c.Params("id")
	if err := DB.Model(&models.NotaPesanan{}).Where("id = ?", id).Update("status", "DIBATALKAN").Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Pesanan berhasil dibatalkan"})
}

// 3. Get Rangkuman Khusus Pesanan (Untuk Tab Rangkuman Bulanan)
func GetRangkumanPesanan(c *fiber.Ctx) error {
	start := c.Query("start")
	end := c.Query("end")

	// Total Global
	var summary struct {
		TotalPendapatan float64 `json:"total_pendapatan"`
		TotalPesanan    int     `json:"total_pesanan"`
	}
	DB.Model(&models.NotaPesanan{}).
		Where("tanggal_kirim >= ? AND tanggal_kirim <= ? AND status != 'DIBATALKAN'", start, end).
		Select("COALESCE(SUM(total_bayar), 0) as total_pendapatan, COUNT(id) as total_pesanan").
		Scan(&summary)

	// Breakdown Per Titik Ambil (Pabrik / Toko A / Toko B)
	var perTitik []struct {
		NamaTitik  string  `json:"nama_titik"`
		Pendapatan float64 `json:"pendapatan"`
		TotalNota  int     `json:"total_nota"`
	}
	DB.Model(&models.NotaPesanan{}).
		Where("tanggal_kirim >= ? AND tanggal_kirim <= ? AND status != 'DIBATALKAN'", start, end).
		Select("nama_toko_snapshot as nama_titik, COALESCE(SUM(total_bayar), 0) as pendapatan, COUNT(id) as total_nota").
		Group("nama_toko_snapshot").
		Order("pendapatan desc").
		Scan(&perTitik)

	// ========================================================
	// BARU: Detail Pesanan per Barang per Titik (Untuk Dropdown)
	// ========================================================
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

// MAIN
func main() {
	connectDB()

	app := fiber.New()

	app.Use(cors.New(cors.Config{
		AllowOrigins: "http://localhost:5173, https://nota-tiara-frontend.vercel.app",
	}))

	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString("Backend Tiara Connected with Env!")
	})

	app.Post("/login", LoginAdmin) // Endpoint Publik (Bisa diakses tanpa token)

	// Kelompokkan rute yang butuh login
	api := app.Group("/api", Protected)

	api.Get("/admins", func(c *fiber.Ctx) error {
		var admins []models.Admin
		// Ambil semua admin, lalu kirim ke Vue
		if err := DB.Find(&admins).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(admins)
	})

	// BARANG
	api.Get("/barangs", GetBarangs)
	api.Put("/barangs/reorder", UpdateUrutanBarang)
	api.Post("/barangs", CreateBarang)
	api.Put("/barangs/:id", UpdateBarang)
	api.Delete("/barangs/:id", DeleteBarang)

	// TOKO
	api.Get("/tokos", GetTokos)
	api.Post("/tokos", CreateToko)
	api.Put("/tokos/:id", UpdateToko)
	api.Delete("/tokos/:id", DeleteToko)

	// NOTA
	api.Get("/profil", GetProfilTiara)
	api.Get("/notas/next-number", GetNextNotaNumber)
	api.Get("/notas", GetNotas)
	api.Get("/notas/:id", GetNotaByID)
	api.Post("/notas", CreateNota)
	api.Put("/notas/:id", UpdateNota)

	// CATATAN BESAR
	api.Get("/catatan-besar", GetCatatanBesar)

	// RANGKUMAN
	api.Get("/rangkuman", GetRangkuman)
	api.Get("/rangkuman-per-toko", GetRangkumanPerToko)

	// SAMPAH
	api.Get("/sampah", GetTrash)
	api.Put("/sampah/:type/:id", RestoreData)

	// NOTA PESANAN (RUTE STATIS DI ATAS)
	api.Get("/pesanan/next-number", GetNextNotaPesananNumber)
	api.Get("/pesanan/catatan", GetCatatanPesanan)
	api.Get("/pesanan/riwayat", GetRiwayatPesanan)
	api.Get("/pesanan/rangkuman-bulanan", GetRangkumanPesanan)

	api.Post("/pesanan", CreateNotaPesanan)

	// NOTA PESANAN (RUTE DINAMIS DENGAN :id WAJIB DI BAWAH)
	api.Get("/pesanan/:id", GetNotaPesananByID)
	api.Post("/pesanan/:id", UpdateNotaPesanan)
	api.Put("/pesanan/:id/batal", BatalkanPesanan)

	// DASHBOARD KUNJUNGAN SALES
	api.Get("/sales/dashboard", GetDashboardSales)
	api.Get("/sales/kunjungan/:toko_id", GetKunjunganToko)

	appPort := os.Getenv("PORT")
	if appPort == "" {
		appPort = "3000"
	}

	log.Println("Server jalan di port " + appPort)
	log.Fatal(app.Listen(":" + appPort))
}
