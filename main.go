package main

import (
	"backend/models"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/swagger"
	"github.com/golang-jwt/jwt/v4"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	_ "backend/docs"
)

var DB *gorm.DB
var jwtSecret []byte

// KONSTANTA ROLE RBAC
const (
	RoleSuperadmin = "superadmin"
	RoleSales      = "sales"
	RoleDapur      = "dapur"
)

// HELPER WIB
func wib() time.Time {
	loc := time.FixedZone("WIB", 7*3600)
	return time.Now().In(loc)
}

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

	// Masukkan variabel ssl ke dalam dsn dan paksa TimeZone Asia/Jakarta
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=Asia/Jakarta",
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
		&models.NotaPembelian{},
		&models.NotaPembelianDetail{},
		&models.Resep{},
		&models.ResepBahan{},
		&models.ProduksiMasak{},
		&models.ProduksiMatang{},
		&models.SisaLayakJual{},
		&models.JurnalEfisiensi{},
		&models.StockOpname{},
		&models.KonversiSatuan{},
		&models.BarangKemasan{},
		&models.BarangRusak{},
		&models.TransaksiKas{},
		&models.PengaturanSistem{},
		&models.AsetSnapshot{},
		&models.NotaPesananDetailKemasan{},
		&models.NotaPesananDetailKomposit{},
		&models.ResepKomposit{},
		&models.ResepKompositDetail{},
		&models.BarangKomposit{},
		&models.KonversiBahan{},
		&models.KonversiBahanDetail{},
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

// LoginAdmin godoc
// @Summary Autentikasi User (Login)
// @Description Endpoint publik untuk mendapatkan JWT Token berdasarkan kredensial Superadmin atau Sales.
// @Tags 01. Authentication
// @Accept json
// @Produce json
// @Param payload body map[string]string true "Format JSON dengan key: username, password"
// @Success 200 {object} map[string]interface{} "Login sukses beserta token"
// @Failure 401 {object} map[string]interface{} "Username atau Password salah"
// @Router /login [post]
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

// MAIN
//
// @title Tiara Bakery Master API
// @version 1.0
// @description Dokumentasi API Internal untuk Sistem Inventory, Nota, dan Kas.
// @contact.name Aristo Budiman
// @host localhost:3000
// @BasePath /
//
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Masukkan token dengan format: "Bearer {token_jwt}"
//
// === DAFTAR ISI SWAGGER (MENGUNCI URUTAN) ===
// @tag.name 01. Authentication
// @tag.name 02. Master Profil
// @tag.name 03. Master Barang
// @tag.name 04. Master Toko
// @tag.name 05. Master Bahan Baku
// @tag.name 06. Master Resep
// @tag.name 07. Master Sampah
// @tag.name 08. Nota Reguler
// @tag.name 09. Nota Pesanan (PO)
// @tag.name 10. Laporan & Rangkuman
// @tag.name 11. Operasional Inventory
// @tag.name 12. Produksi & Dapur
// @tag.name 13. Laporan Penutup (End of Day)
// @tag.name 14. Kas & Keuangan
// @tag.name 15. Analisis & Aset
// @tag.name 16. Distribusi Lapangan
func main() {
	connectDB()

	app := fiber.New()

	// Tambahkan recover agar jika ada panic, server tidak crash dan tetap mengirimkan header CORS
	app.Use(recover.New())

	app.Use(cors.New(cors.Config{
		AllowOrigins: "http://localhost:5173, http://localhost:5174, http://localhost:5175, https://nota-tiara-frontend.vercel.app, https://tiara-inventory.vercel.app, https://kas-tiara.vercel.app",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
		AllowMethods: "GET, POST, HEAD, PUT, DELETE, PATCH, OPTIONS",
	}))

	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString("Backend Tiara Connected with Env!")
	})

	if os.Getenv("APP_ENV") == "development" {
		app.Get("/swagger/*", swagger.New(swagger.Config{
			TagsSorter: "'alpha'",
		}))
		log.Println("⚠️ SWAGGER AKTIF: http://localhost:3000/swagger/index.html")
	}

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
	
	api.Post("/satuan", RequireRole(RoleSuperadmin), CreateKonversiSatuan)
	api.Delete("/satuan/:id", RequireRole(RoleSuperadmin), DeleteKonversiSatuan)

	// PEMBELIAN
	api.Get("/pembelian", RequireRole(RoleSuperadmin), GetPembelianBahan)
	api.Post("/pembelian", RequireRole(RoleSuperadmin), CreatePembelianBahan)
	api.Put("/pembelian/:id/status", RequireRole(RoleSuperadmin), UpdateStatusPembelian)
	api.Delete("/pembelian/:id", RequireRole(RoleSuperadmin), DeletePembelianBahan)
	api.Put("/pembelian/:id/pulihkan", RequireRole(RoleSuperadmin), RestorePembelianBahan)

	// RESEP
	api.Get("/resep", RequireRole(RoleSuperadmin), GetResep)
	api.Post("/resep", RequireRole(RoleSuperadmin), CreateResep)
	api.Put("/resep/:id", RequireRole(RoleSuperadmin), UpdateResep)
	api.Delete("/resep/:id", RequireRole(RoleSuperadmin), DeleteResep)

	// KOMPOSIT
	api.Get("/komposit", RequireRole(RoleSuperadmin), GetKomposit)
	api.Post("/komposit", RequireRole(RoleSuperadmin), CreateKomposit)
	api.Put("/komposit/:id", RequireRole(RoleSuperadmin), UpdateKomposit)
	api.Delete("/komposit/:id", RequireRole(RoleSuperadmin), DeleteKomposit)

	// PRODUKSI HARIAN
	api.Get("/produksi/masak", RequireRole(RoleSuperadmin), GetProduksiMasak)
	api.Post("/produksi/masak", RequireRole(RoleSuperadmin), CreateProduksiMasak)
	api.Delete("/produksi/masak/:id", RequireRole(RoleSuperadmin), DeleteProduksiMasak)
	api.Get("/produksi/matang", RequireRole(RoleSuperadmin), GetProduksiMatang)
	api.Post("/produksi/matang", RequireRole(RoleSuperadmin), CreateProduksiMatang)
	api.Delete("/produksi/matang/:id", RequireRole(RoleSuperadmin), DeleteProduksiMatang)

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
	api.Get("/inventory/pecah-barang", RequireRole(RoleSuperadmin), GetKonversiBahan)
	api.Post("/inventory/pecah-barang", RequireRole(RoleSuperadmin), CreateKonversiBahan)
	api.Delete("/inventory/pecah-barang/:id", RequireRole(RoleSuperadmin), DeleteKonversiBahan)

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
