package main

import (
	"backend/models"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// ==========================================
// HELPER SALDO KAS MASTER
// ==========================================

func TambahSaldoKas(tx *gorm.DB, nominal float64) error {
	var master models.MasterKas
	if err := tx.First(&master, 1).Error; err != nil {
		master = models.MasterKas{ID: 1, Saldo: nominal}
		return tx.Create(&master).Error
	}
	return tx.Model(&master).UpdateColumn("saldo", gorm.Expr("saldo + ?", nominal)).Error
}

func KurangiSaldoKas(tx *gorm.DB, nominal float64) error {
	var master models.MasterKas
	if err := tx.First(&master, 1).Error; err != nil {
		master = models.MasterKas{ID: 1, Saldo: -nominal}
		return tx.Create(&master).Error
	}
	return tx.Model(&master).UpdateColumn("saldo", gorm.Expr("saldo - ?", nominal)).Error
}

// 1. Tarik Data Kas (Bisa difilter per bulan/kategori nanti di Vue)
//
// GetKas godoc
// @Summary Ambil Riwayat Transaksi Kas
// @Description Menarik semua catatan transaksi masuk/keluar dari brankas secara menurun (terbaru di atas).
// @Tags 14. Kas & Keuangan
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {array} models.TransaksiKas "Berhasil menarik data kas"
// @Failure 500 {object} models.ErrorResponse "Kesalahan eksekusi database"
// @Router /api/kas [get]
func GetKas(c *fiber.Ctx) error {
	var kas []models.TransaksiKas
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	query := DB.Order("tanggal desc, id desc")

	if startDate != "" && endDate != "" {
		query = query.Where("DATE(tanggal) >= ? AND DATE(tanggal) <= ?", startDate, endDate)
	}

	if err := query.Find(&kas).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(kas)
}

// 2. Input Kas Manual (Untuk Kategori RUMAH_TANGGA atau Setoran)
//
// CreateKas godoc
// @Summary Catat Transaksi Kas Manual
// @Description Menyimpan transaksi kas secara manual, khusus untuk kategori non-sistem seperti pengeluaran rumah tangga atau setoran modal.
// @Tags 14. Kas & Keuangan
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param payload body models.KasInput true "Data transaksi kas"
// @Success 200 {object} models.MessageResponse "Transaksi kas berhasil dicatat"
// @Failure 400 {object} models.ErrorResponse "Format data JSON salah"
// @Failure 500 {object} models.ErrorResponse "Kesalahan eksekusi database"
// @Router /api/kas [post]
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

	if input.Nominal <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Nominal kas tidak valid. Harus lebih besar dari 0."})
	}

	tgl, _ := time.Parse("2006-01-02", input.Tanggal)
	sekarang := wib()
	if tgl.Format("2006-01-02") == sekarang.Format("2006-01-02") {
		tgl = sekarang // Jika hari ini, gunakan waktu persis detik ini
	} else {
		// Jika hari lain, gunakan jam 23:59:59 agar dihitung full hari itu
		tgl = time.Date(tgl.Year(), tgl.Month(), tgl.Day(), 23, 59, 59, 0, sekarang.Location())
	}
	
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

	tx := DB.Begin()
	if err := tx.Create(&kas).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// Update Saldo Master
	if kas.Jenis == "MASUK" {
		TambahSaldoKas(tx, kas.Nominal)
	} else {
		KurangiSaldoKas(tx, kas.Nominal)
	}
	tx.Commit()

	return c.JSON(fiber.Map{"message": "Transaksi kas berhasil dicatat!", "id": kas.ID})
}

// 3. Hapus Kas (Kalau salah ketik / salah input)
//
// DeleteKas godoc
// @Summary Hapus Transaksi Kas
// @Description Menghapus paksa catatan kas jika terjadi salah ketik atau salah input nominal.
// @Tags 14. Kas & Keuangan
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "ID Kas"
// @Success 200 {object} models.MessageResponse "Transaksi kas berhasil dihapus"
// @Failure 500 {object} models.ErrorResponse "Internal server error"
// @Router /api/kas/{id} [delete]
func DeleteKas(c *fiber.Ctx) error {
	id := c.Params("id")
	tx := DB.Begin()

	// Find dulu untuk mengembalikan saldo
	var kas models.TransaksiKas
	if err := tx.First(&kas, id).Error; err != nil {
		tx.Rollback()
		return c.Status(404).JSON(fiber.Map{"error": "Transaksi kas tidak ditemukan"})
	}

	// Reverse (Kembalikan Saldo)
	if kas.Jenis == "MASUK" {
		KurangiSaldoKas(tx, kas.Nominal)
	} else {
		TambahSaldoKas(tx, kas.Nominal)
	}

	if err := tx.Delete(&kas).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	tx.Commit()
	return c.JSON(fiber.Map{"message": "Transaksi kas berhasil dihapus!"})
}

// PENGATURAN SAKLAR KAS
//
// GetPengaturanKas godoc
// @Summary Cek Status Saklar Kas
// @Description Mengecek apakah fitur sinkronisasi otomatis antara Nota/Pembelian ke Brankas Kas sedang menyala atau mati.
// @Tags 14. Kas & Keuangan
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} models.PengaturanKasResponse "Status sinkronisasi"
// @Router /api/pengaturan/kas [get]
func GetPengaturanKas(c *fiber.Ctx) error {
	var setting models.PengaturanSistem
	DB.Where("key = ?", "ENABLE_KAS_SYNC").First(&setting)

	isActive := setting.Value == "true"
	return c.JSON(fiber.Map{"is_active": isActive})
}

// TogglePengaturanKas godoc
// @Summary Ubah Status Saklar Kas
// @Description Menghidupkan atau mematikan sinkronisasi otomatis (Robot Kas) untuk Nota dan Pembelian.
// @Tags 14. Kas & Keuangan
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param payload body models.ToggleKasInput true "Payload saklar"
// @Success 200 {object} models.MessageResponse "Status sinkronisasi diperbarui"
// @Failure 400 {object} models.ErrorResponse "Format data JSON salah"
// @Router /api/pengaturan/kas [put]
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
//
// GetAnalisisAsetLive godoc
// @Summary Dashboard Aset Live
// @Description Mengkalkulasi total harta kekayaan real-time (Kas, Piutang, Persediaan Gudang) dikurangi Hutang sampai tanggal target.
// @Tags 15. Analisis & Aset
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param date query string false "Tanggal batas akhir kalkulasi (Format: YYYY-MM-DD)"
// @Param start_date query string false "Tanggal mulai untuk hitung total Prive/Pengeluaran RT (Format: YYYY-MM-DD)"
// @Success 200 {object} models.AnalisisAsetResponse "Data live aset berhasil ditarik"
// @Router /api/aset/live [get]
func GetAnalisisAsetLive(c *fiber.Ctx) error {
	// Ambil parameter tanggal dari URL, defaultnya hari ini
	targetDate := c.Query("date")
	startDatePrive := c.Query("start_date") // Tanggal Mulai Hitung Prive

	if targetDate == "" {
		targetDate = wib().Format("2006-01-02")
	}

	tglTarget, _ := time.Parse("2006-01-02", targetDate)

	// Jika start_date kosong, default ke tanggal 1 di bulan yang sama dengan targetDate
	if startDatePrive == "" {
		startDatePrive = time.Date(tglTarget.Year(), tglTarget.Month(), 1, 0, 0, 0, 0, time.Local).Format("2006-01-02")
	}

	// 1. TOTAL KAS (Sampai tanggal target)
	var kasLive float64
	var master models.MasterKas
	DB.First(&master, 1)
	kasLive = master.Saldo

	hariIni := wib().Format("2006-01-02")
	if targetDate != hariIni {
		// Hitung mundur: Kas Dulu = Kas Sekarang - Masuk (Baru) + Keluar (Baru)
		var totalMasukBaru, totalKeluarBaru float64
		DB.Model(&models.TransaksiKas{}).Where("jenis = 'MASUK' AND DATE(tanggal) > ? AND DATE(tanggal) <= ?", targetDate, hariIni).Select("COALESCE(SUM(nominal), 0)").Row().Scan(&totalMasukBaru)
		DB.Model(&models.TransaksiKas{}).Where("jenis = 'KELUAR' AND DATE(tanggal) > ? AND DATE(tanggal) <= ?", targetDate, hariIni).Select("COALESCE(SUM(nominal), 0)").Row().Scan(&totalKeluarBaru)
		kasLive = kasLive - totalMasukBaru + totalKeluarBaru
	}

	// 2. TOTAL PIUTANG (Masih hutang dan dibuat sebelum/pada tanggal target)
	var piutangReguler, piutangPO float64
	DB.Model(&models.Nota{}).Where("is_lunas = false AND status != 'DIBATALKAN' AND tanggal_kirim <= ?", targetDate).Select("COALESCE(SUM(total_bayar), 0)").Row().Scan(&piutangReguler)
	DB.Model(&models.NotaPesanan{}).Where("is_lunas = false AND status != 'DIBATALKAN' AND tanggal_kirim <= ?", targetDate).Select("COALESCE(SUM(sisa_tagihan), 0)").Row().Scan(&piutangPO)
	piutangLive := piutangReguler + piutangPO

	// 3. TOTAL HUTANG (Belum lunas dan dibeli sebelum/pada tanggal target)
	var hutangLive float64
	DB.Model(&models.NotaPembelian{}).Where("is_lunas = false AND tanggal <= ?", targetDate).Select("COALESCE(SUM(total_biaya), 0)").Row().Scan(&hutangLive)

	// 4. NILAI PERSEDIAAN (Menggunakan stok saat ini)
	var inventoryLive float64
	DB.Model(&models.Bahan{}).Select("COALESCE(SUM(stok * harga_saat_ini), 0)").Row().Scan(&inventoryLive)

	// 5. HITUNG PRIVE (Flow: Dari start_date sampai targetDate)
	var totalPrive float64
	DB.Model(&models.TransaksiKas{}).
		Where("kategori = 'RUMAH_TANGGA' AND jenis = 'KELUAR' AND DATE(tanggal) >= ? AND DATE(tanggal) <= ?", startDatePrive, targetDate).
		Select("COALESCE(SUM(nominal), 0)").Row().Scan(&totalPrive)

	// 6. AMBIL DATA BULAN LALU (Snapshot terakhir sebelum targetDate)
	var snapshotLalu models.AsetSnapshot
	DB.Where("bulan < ?", targetDate).Order("bulan desc").First(&snapshotLalu)

	return c.JSON(fiber.Map{
		"live": fiber.Map{
			"total_kas":        kasLive, // Hasil (Masuk - Keluar) s/d targetDate
			"total_piutang":    piutangLive,
			"piutang_reguler":  piutangReguler, // <-- TAMBAHKAN INI
			"piutang_pesanan":  piutangPO,
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
//
// SimpanSnapshotAset godoc
// @Summary Kunci Snapshot Aset (Tutup Bulan)
// @Description Mengunci dan menyimpan nilai kalkulasi aset akhir bulan ke dalam riwayat, agar pertumbuhan kekayaan (profit bersih bulanan) bisa diukur dengan akurat.
// @Tags 15. Analisis & Aset
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param payload body models.SnapshotAsetInput true "Tanggal / Bulan kunci"
// @Success 200 {object} models.MessageResponse "Snapshot berhasil dikunci"
// @Failure 400 {object} models.ErrorResponse "Format tanggal salah"
// @Router /api/aset/snapshot [post]
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
	DB.Model(&models.TransaksiKas{}).Where("jenis = 'MASUK' AND DATE(tanggal) <= ?", tglStr).Select("COALESCE(SUM(nominal), 0)").Row().Scan(&kM)
	DB.Model(&models.TransaksiKas{}).Where("jenis = 'KELUAR' AND DATE(tanggal) <= ?", tglStr).Select("COALESCE(SUM(nominal), 0)").Row().Scan(&kK)

	// 2. PIUTANG (Nota/PO yang dibuat s/d tanggal snapshot dan belum lunas)
	DB.Model(&models.Nota{}).Where("is_lunas = false AND status != 'DIBATALKAN' AND tanggal_kirim <= ?", tglStr).Select("COALESCE(SUM(total_bayar), 0)").Row().Scan(&pR)
	DB.Model(&models.NotaPesanan{}).Where("is_lunas = false AND status != 'DIBATALKAN' AND tanggal_kirim <= ?", tglStr).Select("COALESCE(SUM(sisa_tagihan), 0)").Row().Scan(&pPO)

	// 3. HUTANG (Pembelian s/d tanggal snapshot yang belum lunas)
	DB.Model(&models.NotaPembelian{}).Where("is_lunas = false AND tanggal <= ?", tglStr).Select("COALESCE(SUM(total_biaya), 0)").Row().Scan(&hL)

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

// GetRiwayatAset godoc
// @Summary Riwayat Pertumbuhan Aset
// @Description Menarik riwayat snapshot kekayaan per bulan, diurutkan dari yang terbaru.
// @Tags 15. Analisis & Aset
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {array} models.AsetSnapshot "Data riwayat aset ditarik"
// @Router /api/aset/riwayat [get]
func GetRiwayatAset(c *fiber.Ctx) error {
	var riwayat []models.AsetSnapshot
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	query := DB.Order("bulan desc")

	if startDate != "" && endDate != "" {
		query = query.Where("DATE(bulan) >= ? AND DATE(bulan) <= ?", startDate, endDate)
	}

	query.Find(&riwayat)
	return c.JSON(riwayat)
}
