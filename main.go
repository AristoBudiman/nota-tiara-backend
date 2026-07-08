package main

import (
	"backend/models"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
	_ "time/tzdata" // Embed timezone database to the binary

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
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
		&models.Role{},
		&models.Permission{},
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
		&models.MasterKas{},
	)
	log.Println("Database & Tabel Berhasil Disiapkan! 🏗️")

	// Jalankan SeedRBAC (Migrasi RBAC Dinamis)
	SeedRBAC(DB)

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
			var superRole models.Role
			DB.Where("nama_role = ?", "Superadmin").First(&superRole)

			hashedAdmin, _ := bcrypt.GenerateFromPassword([]byte(adminPass), bcrypt.DefaultCost)
			DB.Create(&models.Admin{
				Username: adminUser,
				Password: string(hashedAdmin),
				RoleID:   superRole.ID,
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
			var salesRole models.Role
			DB.Where("nama_role = ?", "Sales").First(&salesRole)

			hashedSales, _ := bcrypt.GenerateFromPassword([]byte(salesPass), bcrypt.DefaultCost)
			DB.Create(&models.Admin{
				Username: salesUser,
				Password: string(hashedSales),
				RoleID:   salesRole.ID,
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
	if err := DB.Preload("Role").Preload("Role.Permissions").Where("username = ?", input.Username).First(&admin).Error; err != nil {
		time.Sleep(1 * time.Second) // Honeypot delay
		return c.Status(401).JSON(fiber.Map{"error": "Username atau Password salah"})
	}

	// BLOKIR LOGIN MANUAL JIKA AKUN ADALAH AKUN GOOGLE
	if admin.Email != "" {
		time.Sleep(1 * time.Second) // Delay jebakan
		return c.Status(401).JSON(fiber.Map{"error": "Akun ini terdaftar sebagai akun Google. Silakan gunakan tombol Sign in with Google!"})
	}

	// Cek Lockout
	if admin.LockedUntil != nil && admin.LockedUntil.After(time.Now()) {
		return c.Status(403).JSON(fiber.Map{"error": "Akun terkunci karena banyak percobaan gagal. Hubungi Superadmin."})
	}

	// Cek Password Hash vs Password Input
	if err := bcrypt.CompareHashAndPassword([]byte(admin.Password), []byte(input.Password)); err != nil {
		admin.FailedLoginAttempts += 1
		if admin.FailedLoginAttempts >= 5 {
			lockTime := time.Now().Add(24 * time.Hour)
			admin.LockedUntil = &lockTime
		}
		DB.Save(&admin)
		time.Sleep(1 * time.Second) // Honeypot delay
		return c.Status(401).JSON(fiber.Map{"error": "Username atau Password salah"})
	}

	// Reset Lockout jika sukses
	if admin.FailedLoginAttempts > 0 || admin.LockedUntil != nil {
		admin.FailedLoginAttempts = 0
		admin.LockedUntil = nil
		DB.Save(&admin)
	}

	// Ekstrak list permissions
	var perms []string
	for _, p := range admin.Role.Permissions {
		perms = append(perms, p.Kode)
	}

	// Jika sukses, buat token JWT
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"admin_id":    admin.ID,
		"role":        admin.Role.NamaRole, // Pertahankan nama role (opsional untuk frontend)
		"permissions": perms,               // Simpan array permission ke token
		"exp":         time.Now().Add(time.Hour * 24).Unix(), // Berlaku 24 jam
	})

	tokenString, err := token.SignedString(jwtSecret)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal membuat token"})
	}

	return c.JSON(fiber.Map{
		"message":     "Login sukses",
		"token":       tokenString,
		"role":        admin.Role.NamaRole,
		"permissions": perms,
	})
}

// LoginGoogle godoc
// @Summary Autentikasi User via Google Sign-In
// @Description Endpoint untuk login menggunakan ID Token dari Google.
// @Tags 01. Authentication
// @Accept json
// @Produce json
// @Param payload body map[string]string true "Format JSON dengan key: token"
// @Success 200 {object} map[string]interface{} "Login sukses beserta token"
// @Failure 401 {object} map[string]interface{} "Akses Ditolak"
// @Router /login/google [post]
func LoginGoogle(c *fiber.Ctx) error {
	var input struct {
		Token string `json:"token"`
	}

	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Input tidak valid"})
	}

	// 1. Verifikasi Token ke Google
	resp, err := http.Get("https://oauth2.googleapis.com/tokeninfo?id_token=" + input.Token)
	if err != nil || resp.StatusCode != http.StatusOK {
		return c.Status(401).JSON(fiber.Map{"error": "Token Google tidak valid"})
	}
	defer resp.Body.Close()

	var googleClaims struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&googleClaims); err != nil || googleClaims.Email == "" {
		return c.Status(401).JSON(fiber.Map{"error": "Gagal membaca email dari Google"})
	}

	// 2. Cari Email di Database (Whitelist)
	var admin models.Admin
	if err := DB.Preload("Role").Preload("Role.Permissions").Where("email = ?", googleClaims.Email).First(&admin).Error; err != nil {
		// Logika Bypass: Akun terkunci tidak masalah untuk Google Login!
		return c.Status(401).JSON(fiber.Map{"error": "Akses Ditolak. Email Anda belum didaftarkan oleh Superadmin."})
	}

	// Ekstrak list permissions
	var perms []string
	for _, p := range admin.Role.Permissions {
		perms = append(perms, p.Kode)
	}

	// Jika sukses, buat token JWT aplikasi kita
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"admin_id":    admin.ID,
		"role":        admin.Role.NamaRole,
		"permissions": perms,
		"exp":         time.Now().Add(time.Hour * 24).Unix(),
	})

	tokenString, err := token.SignedString(jwtSecret)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal membuat token"})
	}

	return c.JSON(fiber.Map{
		"message":     "Login Google sukses",
		"token":       tokenString,
		"role":        admin.Role.NamaRole,
		"permissions": perms,
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
	
	if roleStr, ok := claims["role"].(string); ok {
		c.Locals("role", roleStr)
	}

	if permsIfc, ok := claims["permissions"].([]interface{}); ok {
		var perms []string
		for _, p := range permsIfc {
			perms = append(perms, p.(string))
		}
		c.Locals("permissions", perms)
	}

	return c.Next()
}

// MIDDLEWARE RBAC BARU (Menggunakan Permissions)
func RequirePermission(requiredPermission string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Bypass Superadmin (Dewa)
		roleStr, ok := c.Locals("role").(string)
		if ok && roleStr == "Superadmin" {
			return c.Next() // Bypass all
		}

		userPerms, ok := c.Locals("permissions").([]string)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Akses ditolak. Tidak ada data hak akses.",
			})
		}

		hasAccess := false
		for _, p := range userPerms {
			if p == requiredPermission {
				hasAccess = true
				break
			}
		}

		if !hasAccess {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "Akses ditolak. Anda tidak memiliki izin ini (" + requiredPermission + ").",
			})
		}

		return c.Next()
	}
}

// MIDDLEWARE RBAC BARU (Multiple Permissions dengan logika OR)
func RequirePermissionAny(requiredPermissions ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		roleStr, ok := c.Locals("role").(string)
		if ok && roleStr == "Superadmin" {
			return c.Next()
		}

		userPerms, ok := c.Locals("permissions").([]string)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Akses ditolak. Tidak ada data hak akses.",
			})
		}

		hasAccess := false
		for _, required := range requiredPermissions {
			for _, p := range userPerms {
				if p == required {
					hasAccess = true
					break
				}
			}
			if hasAccess {
				break
			}
		}

		if !hasAccess {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "Anda tidak memiliki satupun hak dari yang dibutuhkan.",
			})
		}
		return c.Next()
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

	// Limitasi Login (Maks 5 per menit)
	loginLimiter := limiter.New(limiter.Config{
		Max:        5,
		Expiration: 1 * time.Minute,
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "Terlalu banyak percobaan login. Silakan tunggu 1 menit.",
			})
		},
	})
	app.Post("/login", loginLimiter, LoginAdmin) // Endpoint Publik (Honeypot/Fallback)
	app.Post("/login/google", loginLimiter, LoginGoogle) // Endpoint Google Login

	// Kelompokkan rute yang butuh login
	api := app.Group("/api", Protected)

	// Profil (Dapat diakses oleh semua Role yang sudah login)
	api.Get("/profile", GetProfile)
	api.Put("/profile", UpdateProfile)

	api.Get("/admins", func(c *fiber.Ctx) error {
		var admins []models.Admin
		// Ambil semua admin, lalu kirim ke Vue (Sembunyikan Password)
		if err := DB.Preload("Role").Select("id, username, email, role_id, failed_login_attempts, locked_until").Find(&admins).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(admins)
	})
	api.Post("/admins", RequirePermission("manage_admin"), CreateAdmin)
	api.Put("/admins/:id", RequirePermission("manage_admin"), UpdateAdmin)
	api.Delete("/admins/:id", RequirePermission("manage_admin"), DeleteAdmin)

	// ROLES & PERMISSIONS
	api.Get("/roles", RequirePermission("manage_admin"), GetRoles)
	api.Post("/roles", RequirePermission("manage_admin"), CreateRole)
	api.Put("/roles/:id", RequirePermission("manage_admin"), UpdateRole)
	api.Delete("/roles/:id", RequirePermission("manage_admin"), DeleteRole)
	
	api.Get("/permissions", RequirePermission("manage_admin"), GetPermissions)

	// BAHAN
	api.Get("/bahan", GetBahan)
	api.Put("/bahan/reorder", RequirePermission("manage_master_bahan"), UpdateUrutanBahan)
	api.Post("/bahan", RequirePermission("manage_master_bahan"), CreateBahan)
	api.Put("/bahan/:id", RequirePermission("manage_master_bahan"), UpdateBahan)
	api.Delete("/bahan/:id", RequirePermission("manage_master_bahan"), DeleteBahan)
	
	api.Post("/satuan", RequirePermission("manage_master_bahan"), CreateKonversiSatuan)
	api.Delete("/satuan/:id", RequirePermission("manage_master_bahan"), DeleteKonversiSatuan)

	// PEMBELIAN
	api.Get("/pembelian", RequirePermission("manage_pembelian"), GetPembelianBahan)
	api.Post("/pembelian", RequirePermission("manage_pembelian"), CreatePembelianBahan)
	api.Put("/pembelian/:id/status", RequirePermission("manage_pembelian"), UpdateStatusPembelian)
	api.Delete("/pembelian/:id", RequirePermission("manage_pembelian"), DeletePembelianBahan)
	api.Put("/pembelian/:id/pulihkan", RequirePermission("manage_pembelian"), RestorePembelianBahan)

	// RESEP
	api.Get("/resep", RequirePermission("manage_resep"), GetResep)
	api.Post("/resep", RequirePermission("manage_resep"), CreateResep)
	api.Put("/resep/:id", RequirePermission("manage_resep"), UpdateResep)
	api.Delete("/resep/:id", RequirePermission("manage_resep"), DeleteResep)

	// KOMPOSIT
	api.Get("/komposit", RequirePermission("manage_komposit"), GetKomposit)
	api.Post("/komposit", RequirePermission("manage_komposit"), CreateKomposit)
	api.Put("/komposit/:id", RequirePermission("manage_komposit"), UpdateKomposit)
	api.Delete("/komposit/:id", RequirePermission("manage_komposit"), DeleteKomposit)

	// PRODUKSI HARIAN
	api.Get("/produksi/masak", RequirePermission("manage_produksi_masak"), GetProduksiMasak)
	api.Post("/produksi/masak", RequirePermission("manage_produksi_masak"), CreateProduksiMasak)
	api.Delete("/produksi/masak/:id", RequirePermission("manage_produksi_masak"), DeleteProduksiMasak)
	api.Get("/produksi/matang", RequirePermission("manage_produksi_matang"), GetProduksiMatang)
	api.Post("/produksi/matang", RequirePermission("manage_produksi_matang"), CreateProduksiMatang)
	api.Delete("/produksi/matang/:id", RequirePermission("manage_produksi_matang"), DeleteProduksiMatang)

	// AFKIR / BARANG RUSAK
	api.Get("/inventory/rusak", RequirePermission("manage_barang_rusak"), GetBarangRusak)
	api.Post("/inventory/rusak", RequirePermission("manage_barang_rusak"), CreateBarangRusak)
	api.Delete("/inventory/rusak/:id", RequirePermission("manage_barang_rusak"), DeleteBarangRusak)

	// TUTUP BUKU & OPNAME
	api.Post("/produksi/tutup-buku", RequirePermission("manage_tutup_buku"), TutupBukuHarian)
	api.Get("/produksi/jurnal", RequirePermission("view_jurnal_dapur"), GetJurnalTutupBuku)
	api.Get("/opname", RequirePermission("manage_opname"), GetOpname)
	api.Post("/opname", RequirePermission("manage_opname"), CreateOpname)
	api.Get("/konversi/sisa-kemarin", RequirePermission("view_jurnal_dapur"), GetSisaLayakJualKemarin)
	api.Get("/inventory/pecah-barang", RequirePermission("manage_pecah_barang"), GetKonversiBahan)
	api.Post("/inventory/pecah-barang", RequirePermission("manage_pecah_barang"), CreateKonversiBahan)
	api.Delete("/inventory/pecah-barang/:id", RequirePermission("manage_pecah_barang"), DeleteKonversiBahan)

	// BARANG
	api.Get("/barangs", GetBarangs)
	api.Put("/barangs/reorder", RequirePermission("manage_master_barang"), UpdateUrutanBarang)
	api.Post("/barangs", RequirePermission("manage_master_barang"), CreateBarang)
	api.Put("/barangs/:id", RequirePermission("manage_master_barang"), UpdateBarang)
	api.Delete("/barangs/:id", RequirePermission("manage_master_barang"), DeleteBarang)

	// TOKO
	api.Get("/tokos", GetTokos)
	api.Post("/tokos", RequirePermission("manage_master_toko"), CreateToko)
	api.Put("/tokos/:id", RequirePermission("manage_master_toko"), UpdateToko)
	api.Delete("/tokos/:id", RequirePermission("manage_master_toko"), DeleteToko)

	// NOTA
	api.Get("/profil", GetProfilTiara)
	api.Get("/notas/next-number", RequirePermission("manage_nota_jual"), GetNextNotaNumber)
	api.Get("/notas", RequirePermission("view_riwayat_nota"), GetNotas)
	api.Get("/notas/:id", RequirePermission("manage_nota_jual"), GetNotaByID)
	api.Post("/notas", RequirePermission("manage_nota_jual"), CreateNota)
	api.Put("/notas/:id", RequirePermission("manage_nota_jual"), UpdateNota)
	api.Put("/notas/:id/batal", RequirePermission("manage_nota_jual"), BatalkanNota)
	api.Put("/notas/:id/pulihkan", RequirePermission("manage_nota_jual"), PulihkanNota)

	// CATATAN BESAR
	api.Get("/catatan-besar", RequirePermission("view_catatan_besar"), GetCatatanBesar)

	// RANGKUMAN
	api.Get("/rangkuman", RequirePermission("view_rangkuman_penjualan"), GetRangkuman)
	api.Get("/rangkuman-per-toko", RequirePermission("view_rangkuman_penjualan"), GetRangkumanPerToko)

	// KAS
	api.Get("/kas", RequirePermission("manage_kas"), GetKas)
	api.Post("/kas", RequirePermission("manage_kas"), CreateKas)
	api.Delete("/kas/:id", RequirePermission("manage_kas"), DeleteKas)

	// SAKLAR KAS (TAMBAHKAN INI)
	api.Get("/pengaturan/kas", RequirePermission("manage_saklar_kas"), GetPengaturanKas)
	api.Put("/pengaturan/kas", RequirePermission("manage_saklar_kas"), TogglePengaturanKas)

	// SAMPAH
	api.Get("/sampah", RequirePermission("manage_sampah"), GetTrash)
	api.Put("/sampah/:type/:id", RequirePermission("manage_sampah"), RestoreData)

	// NOTA PESANAN (RUTE STATIS DI ATAS)
	api.Get("/pesanan/next-number", RequirePermission("manage_nota_pesanan"), GetNextNotaPesananNumber)
	api.Get("/pesanan/catatan", RequirePermission("manage_nota_pesanan"), GetCatatanPesanan)
	api.Get("/pesanan/riwayat", RequirePermission("view_riwayat_pesanan"), GetRiwayatPesanan)
	api.Get("/pesanan/rangkuman-bulanan", RequirePermission("view_rangkuman_pesanan"), GetRangkumanPesanan)

	// NOTA PESANAN (RUTE DINAMIS DENGAN :id WAJIB DI BAWAH)
	api.Post("/pesanan", RequirePermission("manage_nota_pesanan"), CreateNotaPesanan)
	api.Get("/pesanan/:id", RequirePermissionAny("manage_nota_pesanan", "view_riwayat_pesanan"), GetNotaPesananByID)
	api.Post("/pesanan/:id", RequirePermission("manage_nota_pesanan"), UpdateNotaPesanan)
	api.Put("/pesanan/:id/batal", RequirePermission("manage_nota_pesanan"), BatalkanPesanan)
	api.Put("/pesanan/:id/pulihkan", RequirePermission("manage_nota_pesanan"), PulihkanPesanan)

	// DASHBOARD KUNJUNGAN SALES
	api.Get("/sales/dashboard", RequirePermission("view_dashboard_sales"), GetDashboardSales)
	api.Get("/sales/kunjungan/:toko_id", RequirePermission("view_dashboard_sales"), GetKunjunganToko)

	// ANALISIS ASET & PERTUMBUHAN
	api.Get("/aset/live", RequirePermission("view_analisis_aset"), GetAnalisisAsetLive)
	api.Get("/aset/rincian", RequirePermission("view_analisis_aset"), GetRincianAset)
	api.Get("/aset/rincian-kas", RequirePermission("view_analisis_aset"), GetRincianMutasiKas)
	api.Post("/aset/snapshot", RequirePermission("view_analisis_aset"), SimpanSnapshotAset)
	api.Get("/aset/riwayat", RequirePermission("view_analisis_aset"), GetRiwayatAset)

	appPort := os.Getenv("PORT")
	if appPort == "" {
		appPort = "3000"
	}

	log.Println("Server jalan di port " + appPort)
	log.Fatal(app.Listen(":" + appPort))
}
