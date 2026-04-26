package main

import (
	"backend/models"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
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

	// Ambil setting SSL dari .env, kalau kosong anggap disable (untuk lokal lama)
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
	)
	log.Println("Database & Tabel Berhasil Disiapkan! 🏗️")

	// Buat Akun Super Admin Default jika tabel kosong
	var count int64
	DB.Model(&models.Admin{}).Count(&count)
	if count == 0 {
		hashedPassword, _ := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
		DB.Create(&models.Admin{
			Username: "admin",
			Password: string(hashedPassword),
			Role:     "superadmin",
		})
		log.Println("✅ Akun Super Admin default berhasil dibuat! (User: admin | Pass: admin123)")
	}
}

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
	})
}

func Protected(c *fiber.Ctx) error {
	// Ambil header Authorization: Bearer <token>
	authHeader := c.Get("Authorization")
	if authHeader == "" || len(authHeader) < 8 {
		return c.Status(401).JSON(fiber.Map{"error": "Akses ditolak. Token tidak ada."})
	}

	tokenString := authHeader[7:] // Potong tulisan "Bearer "

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})

	if err != nil || !token.Valid {
		return c.Status(401).JSON(fiber.Map{"error": "Token tidak valid atau sudah kedaluwarsa"})
	}

	return c.Next() // Lanjut ke fungsi asli
}

func CreateNota(c *fiber.Ctx) error {
	var input struct {
		NoNota       string `json:"no_nota"`
		TokoID       uint   `json:"toko_id"`
		TanggalKirim string `json:"tanggal_kirim"`
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

	// 2. Logika Penentuan Siklus Snapshot
	var siklusAktif string

	if toko.IsHarian {
		siklusAktif = "HARIAN"
	} else {
		// Menggunakan Switch Condition untuk Toko Reguler
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

	nota := models.Nota{
		NoNota:           input.NoNota,
		TokoID:           input.TokoID,
		TanggalKirim:     tgl,
		Status:           "KIRIM",
		NamaTokoSnapshot: toko.NamaToko,
		SiklusSnapshot:   siklusAktif, // Terkunci rapi sesuai jadwal kirim
		IsHarianSnapshot: toko.IsHarian,
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

// func UpdateNota(c *fiber.Ctx) error {
// 	id := c.Params("id")
// 	var input struct {
// 		Details []struct {
// 			ID          uint `json:"id"`
// 			BanyakRetur int  `json:"banyak_retur"`
// 		} `json:"details"`
// 	}
// 	if err := c.BodyParser(&input); err != nil {
// 		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
// 	}

// 	for _, d := range input.Details {
// 		// Cari detail spesifik berdasarkan ID-nya
// 		var detail models.NotaDetail
// 		if err := DB.First(&detail, d.ID).Error; err == nil {
// 			hRetur := float64(d.BanyakRetur) * detail.HargaJual
// 			DB.Model(&detail).Updates(map[string]interface{}{
// 				"banyak_retur": d.BanyakRetur,
// 				"harga_retur":  hRetur,
// 			})
// 		}
// 	}

// 	// Hitung ulang total di Header Nota
// 	var totalKirim, totalRetur float64
// 	DB.Model(&models.NotaDetail{}).Where("nota_id = ?", id).Select("SUM(harga_kirim)").Row().Scan(&totalKirim)
// 	DB.Model(&models.NotaDetail{}).Where("nota_id = ?", id).Select("SUM(harga_retur)").Row().Scan(&totalRetur)

// 	DB.Model(&models.Nota{}).Where("id = ?", id).Updates(map[string]interface{}{
// 		"jumlah_retur": totalRetur,
// 		"total_bayar":  totalKirim - totalRetur,
// 	})

// 	return c.JSON(fiber.Map{"message": "Retur berhasil disimpan!"})
// }

func UpdateNota(c *fiber.Ctx) error {
	id := c.Params("id")
	var input struct {
		Details []struct {
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
			// Kita harus buat baris baru di nota_details
			var barang models.Barang
			DB.First(&barang, d.BarangID)

			parsedID, err := strconv.Atoi(id)
			if err != nil {
				return c.Status(400).JSON(fiber.Map{"error": "ID Nota tidak valid"})
			}

			newDetail := models.NotaDetail{
				NotaID:             uint(parsedID), // Pastikan ID Nota benar
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
	// DB.Model(&models.NotaDetail{}).Where("nota_id = ?", id).Select("SUM(harga_kirim)").Row().Scan(&totalKirim)
	// DB.Model(&models.NotaDetail{}).Where("nota_id = ?", id).Select("SUM(harga_retur)").Row().Scan(&totalRetur)

	DB.Model(&models.Nota{}).Where("id = ?", id).Updates(map[string]interface{}{
		"jumlah_retur": totalRetur,
		"total_bayar":  totalKirim - totalRetur,
	})

	return c.JSON(fiber.Map{"message": "Retur berhasil diperbarui!"})
}

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

func GetBarangs(c *fiber.Ctx) error {
	var barangs []models.Barang
	// UBAH DI SINI: Tambahkan .Order() agar posisinya terkunci permanen!
	// Jika urutannya sama (0), akan diurutkan dari ID paling lama.
	if err := DB.Order("urutan asc, id asc").Find(&barangs).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(barangs)
}

// FUNGSI BARU UNTUK MENYIMPAN URUTAN PLAYLIST
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

func GetTokos(c *fiber.Ctx) error {
	var tokos []models.Toko
	if err := DB.Find(&tokos).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(tokos)
}

func GetNotas(c *fiber.Ctx) error {
	var notas []models.Nota
	// Gunakan "id desc" agar nota yang baru dibuat muncul paling atas
	if err := DB.Preload("Toko").Preload("Details").Preload("Details.Barang").Order("id desc").Find(&notas).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(notas)
}

func GetNotaByID(c *fiber.Ctx) error {
	id := c.Params("id")
	var nota models.Nota
	// Tambahkan .Order("id ASC") pada Preload Details
	if err := DB.Preload("Toko").Preload("Details", func(db *gorm.DB) *gorm.DB {
		return db.Order("nota_details.id ASC")
	}).First(&nota, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Nota tidak ditemukan"})
	}
	return c.JSON(nota)
}

// --- FUNGSI CRUD TOKO ---
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

func DeleteToko(c *fiber.Ctx) error {
	id := c.Params("id")
	if err := DB.Delete(&models.Toko{}, id).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Toko berhasil dihapus"})
}

// --- FUNGSI CRUD BARANG ---
func CreateBarang(c *fiber.Ctx) error {
	var input models.Barang
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if err := DB.Create(&input).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(input)
}

func DeleteBarang(c *fiber.Ctx) error {
	id := c.Params("id")
	if err := DB.Delete(&models.Barang{}, id).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Barang berhasil dihapus"})
}

// --- FUNGSI UPDATE TOKO ---
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

	// PERBAIKAN: Gunakan Map agar nilai 'false' tetap terbaca dan di-update ke database
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

// --- FUNGSI UPDATE BARANG ---
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

func GetRangkuman(c *fiber.Ctx) error {
	start := c.Query("start")
	end := c.Query("end")

	if start == "" || end == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Tanggal start dan end wajib diisi"})
	}

	startDate, errStart := time.Parse("2006-01-02", start)
	endDate, errEnd := time.Parse("2006-01-02", end)
	if errStart != nil || errEnd != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format tanggal salah"})
	}

	// 1. AMBIL SEMUA TOKO DARI MASTER DATA
	var semuaToko []models.Toko
	if err := DB.Unscoped().Find(&semuaToko).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal mengambil data master toko"})
	}

	// 2. BUAT KERANGKA REKAP DENGAN MAP (Semua toko di-set 0)
	rekapMap := make(map[uint]*models.RekapToko)
	for _, t := range semuaToko {
		rekapMap[t.ID] = &models.RekapToko{
			ID:         t.ID,
			Nama:       t.NamaToko,
			Kirim:      0,
			Retur:      0,
			Pendapatan: 0,
			Persentase: 0,
		}
	}

	var rawResults []struct {
		ID    uint
		Nama  string
		Kirim float64
		Retur float64
	}

	// 3. EKSEKUSI SQL
	query := `
		SELECT 
			toko_id as id,
			MAX(nama_toko_snapshot) as nama,
			COALESCE(SUM(CASE WHEN tanggal_kirim >= CAST(? AS DATE) AND tanggal_kirim <= CAST(? AS DATE) THEN jumlah_kirim ELSE 0 END), 0) as kirim,
			COALESCE(SUM(CASE 
				WHEN siklus_snapshot = 'HARIAN' AND tanggal_kirim >= CAST(? AS DATE) AND tanggal_kirim <= CAST(? AS DATE) THEN jumlah_retur
				WHEN siklus_snapshot != 'HARIAN' AND (tanggal_kirim + INTERVAL '4 days') >= CAST(? AS DATE) AND (tanggal_kirim + INTERVAL '4 days') <= CAST(? AS DATE) THEN jumlah_retur
				ELSE 0 
			END), 0) as retur
		FROM nota
		WHERE tanggal_kirim >= CAST(? AS DATE) - INTERVAL '4 days' AND tanggal_kirim <= CAST(? AS DATE)
		GROUP BY toko_id
	`

	if err := DB.Raw(query, startDate, endDate, startDate, endDate, startDate, endDate, startDate, endDate).Scan(&rawResults).Error; err != nil {
		fmt.Println("❌ ERROR SQL RANGKUMAN:", err.Error())
		return c.Status(500).JSON(fiber.Map{"error": "Gagal menghitung laporan: " + err.Error()})
	}

	// 4. TIMPA KERANGKA MAP DENGAN HASIL TRANSAKSI
	var totalKirim, totalRetur float64

	for _, r := range rawResults {
		if val, exists := rekapMap[r.ID]; exists {
			val.Kirim = r.Kirim
			val.Retur = r.Retur
			val.Pendapatan = r.Kirim - r.Retur

			if r.Kirim > 0 {
				val.Persentase = (r.Retur / r.Kirim) * 100
			}
			// Update dengan nama snapshot jika ada transaksi
			if r.Nama != "" {
				val.Nama = r.Nama
			}
		}
		totalKirim += r.Kirim
		totalRetur += r.Retur
	}

	// 5. UBAH MAP MENJADI ARRAY SLICE DAN URUTKAN SESUAI ABJAD
	var perToko []models.RekapToko
	for _, r := range rekapMap {
		perToko = append(perToko, *r)
	}

	// Mengurutkan nama toko dari A ke Z
	sort.Slice(perToko, func(i, j int) bool {
		return perToko[i].Pendapatan > perToko[j].Pendapatan
	})

	totalPersentase := 0.0
	if totalKirim > 0 {
		totalPersentase = (totalRetur / totalKirim) * 100
	}

	return c.JSON(models.RangkumanResponse{
		Kirim:      totalKirim,
		Retur:      totalRetur,
		Pendapatan: totalKirim - totalRetur,
		Persentase: totalPersentase,
		PerToko:    perToko,
	})
}

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

	api.Get("/tokos", GetTokos)
	api.Post("/tokos", CreateToko)

	api.Get("/barangs", GetBarangs)
	api.Get("/tokos", GetTokos)
	api.Get("/notas", GetNotas)
	api.Get("/notas/:id", GetNotaByID)
	api.Put("/notas/:id", UpdateNota)
	api.Post("/notas", CreateNota)
	api.Get("/catatan-besar", GetCatatanBesar)
	api.Put("/barangs/reorder", UpdateUrutanBarang)
	api.Post("/barangs", CreateBarang)
	api.Put("/barangs/:id", UpdateBarang)
	api.Delete("/barangs/:id", DeleteBarang)
	api.Post("/tokos", CreateToko)
	api.Put("/tokos/:id", UpdateToko)
	api.Delete("/tokos/:id", DeleteToko)
	api.Get("/rangkuman", GetRangkuman)

	appPort := os.Getenv("PORT")
	if appPort == "" {
		appPort = "3000"
	}

	log.Println("Server jalan di port " + appPort)
	log.Fatal(app.Listen(":" + appPort))
}
