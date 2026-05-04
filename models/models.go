package models

import (
	"time"

	"gorm.io/gorm"
)

// 1. IDENTITAS TIARA (Header Nota)
type ProfilTiara struct {
	ID       uint   `gorm:"primaryKey"`
	Nama     string `gorm:"default:'Tiara'"`
	LogoPath string
	Alamat   string
	NoTelp   string
	NoHP     string
}

// 2. MASTER TOKO (Mitra)
type Toko struct {
	ID uint `gorm:"primaryKey"`

	// SOFT DELETE
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	NamaToko string `gorm:"not null"`
	Alamat   string
	NoTelp   string
	// Flag Siklus
	SiklusKamisSenin  bool `gorm:"default:false"`
	SiklusJumatSelasa bool `gorm:"default:false"`
	SiklusSabtuRabu   bool `gorm:"default:false"`
	IsHarian          bool `gorm:"default:false" json:"IsHarian"`
}

// 3. MASTER BARANG
type Barang struct {
	ID uint `gorm:"primaryKey"`

	// SOFT DELETE
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	NamaBarang   string `gorm:"not null"`
	HargaDefault float64

	Urutan int `gorm:"default:0" json:"Urutan"`

	// TAMBAHAN MODUL INVENTORY
	ResepID         *uint   `json:"resep_id"` // Bisa NULL jika belum di-link
	Resep           *Resep  `gorm:"foreignKey:ResepID" json:"resep,omitempty"`
	MetodeKonversi  string  `gorm:"default:'Gram'" json:"metode_konversi"` // "Gram" atau "Pcs"
	KebutuhanAdonan float64 `gorm:"default:0" json:"kebutuhan_adonan"`     // Berapa gram / fraksi per 1 roti
	MasaSimpan      int     `gorm:"default:2" json:"masa_simpan"`          // Default 2 hari

	Kemasan []BarangKemasan `gorm:"foreignKey:BarangID" json:"kemasan_detail"`
}

// 4. HEADER NOTA
type Nota struct {
	ID           uint      `gorm:"primaryKey"`
	NoNota       string    `gorm:"unique;not null"`
	TokoID       uint      `gorm:"not null"`
	Toko         Toko      `gorm:"foreignKey:TokoID"`
	TanggalKirim time.Time `gorm:"type:date"`

	// SNAPSHOT UNTUK MENGUNCI SEJARAH
	NamaTokoSnapshot string `json:"NamaTokoSnapshot"`
	SiklusSnapshot   string `json:"SiklusSnapshot"`
	IsHarianSnapshot bool   `gorm:"default:false" json:"IsHarianSnapshot"`

	// Hasil Perhitungan
	JumlahKirim float64 `gorm:"default:0"` // Total harga kirim (Semua barang)
	JumlahRetur float64 `gorm:"default:0"` // Total harga retur (Semua barang)
	TotalBayar  float64 `gorm:"default:0"` // JumlahKirim - JumlahRetur

	// PELACAK SALES
	CreatedBy  uint      `json:"created_by"`
	AssignedTo uint      `json:"assigned_to"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`

	Status  string `gorm:"default:'KIRIM'"`
	IsLunas bool   `gorm:"default:false" json:"is_lunas"` // 'KIRIM' atau 'SELESAI'
	Details []NotaDetail
}

// 5. DETAIL BARANG DALAM NOTA (Isi Tabel Nota)
type NotaDetail struct {
	ID       uint   `gorm:"primaryKey"`
	NotaID   uint   `gorm:"not null"`
	BarangID uint   `gorm:"not null"`
	Barang   Barang `gorm:"foreignKey:BarangID"`

	// SNAPSHOT UNTUK MENGUNCI SEJARAH
	NamaBarangSnapshot string `json:"NamaBarangSnapshot"`

	BanyakKirim int     `gorm:"default:0"`
	HargaJual   float64 `gorm:"not null"`
	HargaKirim  float64 `gorm:"default:0"` // BanyakKirim * HargaJual

	BanyakRetur int     `gorm:"default:0"`
	HargaRetur  float64 `gorm:"default:0"` // BanyakRetur * HargaJual
}

type Admin struct {
	ID       uint   `gorm:"primaryKey"`
	Username string `gorm:"unique;not null"`
	Password string `gorm:"not null"`
	Role     string `gorm:"default:'superadmin'"`
}

type RekapToko struct {
	ID         uint    `json:"id"`
	Nama       string  `json:"nama"`
	Kirim      float64 `json:"kirim"`
	Retur      float64 `json:"retur"`
	Pendapatan float64 `json:"pendapatan"`
	Persentase float64 `json:"persentase"`
}

type RekapBarang struct {
	Nama       string  `json:"nama"`
	QtyKirim   float64 `json:"qty_kirim"`
	QtyRetur   float64 `json:"qty_retur"`
	QtyLaku    float64 `json:"qty_laku"`
	Persentase float64 `json:"persentase"`
}

type RangkumanResponse struct {
	Kirim      float64       `json:"kirim"`
	Retur      float64       `json:"retur"`
	Pendapatan float64       `json:"pendapatan"`
	Persentase float64       `json:"persentase"`
	PerToko    []RekapToko   `json:"perToko"`
	PerBarang  []RekapBarang `json:"perBarang"`
}

// HEADER NOTA PESANAN
type NotaPesanan struct {
	ID           uint      `gorm:"primaryKey"`
	NoNota       string    `gorm:"unique;not null"`
	NamaPemesan  string    `gorm:"not null"`
	TanggalKirim time.Time `gorm:"type:date"`

	JenisPengambilan string `gorm:"default:'PABRIK'"` // 'PABRIK' atau 'MITRA'

	// Gunakan pointer (*uint) agar bisa bernilai NULL di database jika diambil di Pabrik
	TokoID           *uint
	Toko             Toko   `gorm:"foreignKey:TokoID"`
	NamaTokoSnapshot string `json:"NamaTokoSnapshot"` // Catat nama toko saat itu (atau isi "PABRIK")

	TotalBayar float64 `gorm:"default:0"`
	Status     string  `gorm:"default:'BELUM DIAMBIL'"`
	IsLunas    bool    `gorm:"default:false" json:"is_lunas"` // 'BELUM DIAMBIL' atau 'LUNAS/DIAMBIL'

	AssignedTo uint      `json:"assigned_to"`
	CreatedBy  uint      `json:"created_by"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`

	Details []NotaPesananDetail `gorm:"foreignKey:NotaPesananID"`
}

// DETAIL BARANG PESANAN
type NotaPesananDetail struct {
	ID            uint `gorm:"primaryKey"`
	NotaPesananID uint `gorm:"not null"`

	// Pointer agar bisa NULL untuk barang kustom yang tidak ada di Master Barang
	BarangID *uint
	Barang   Barang `gorm:"foreignKey:BarangID"`

	// Ini menyimpan nama barang dari DB, ATAU nama barang kustom ketikan manual (misal: "Kue Tart")
	NamaBarangBebas string `gorm:"not null" json:"NamaBarangBebas"`

	// Tambahan untuk persiapan Modul Inventory Dapur
	ResepID *uint   `json:"resep_id"` // Bisa NULL
	Gramasi float64 `gorm:"default:0" json:"gramasi"`

	Banyak    int     `gorm:"default:0"`
	HargaJual float64 `gorm:"not null"`
	Subtotal  float64 `gorm:"default:0"` // Banyak * HargaJual
}

// ============================================================================
// MODUL INVENTORY
// ============================================================================

// 6. MASTER BAHAN BAKU
type Bahan struct {
	ID           uint           `gorm:"primaryKey"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
	NamaBahan    string         `gorm:"not null" json:"nama_bahan"`
	Satuan       string         `gorm:"not null" json:"satuan"` // gr, ml, pcs
	Stok         float64        `gorm:"default:0" json:"stok"`
	HargaSaatIni float64        `gorm:"default:0" json:"harga_saat_ini"` // Update otomatis dari pembelian terakhir
	BatasMinimum float64        `gorm:"default:0" json:"batas_minimum"`
}

type BarangKemasan struct {
	ID        uint    `gorm:"primaryKey"`
	BarangID  uint    `gorm:"not null" json:"barang_id"`
	BahanID   uint    `gorm:"not null" json:"bahan_id"`
	Bahan     Bahan   `gorm:"foreignKey:BahanID" json:"bahan"`
	Kebutuhan float64 `gorm:"not null" json:"kebutuhan"` // (Bisa pecahan, misal 0.25)
}

// 7. RIWAYAT BELANJA (PEMBELIAN BAHAN)
type PembelianBahan struct {
	ID              uint      `gorm:"primaryKey"`
	Tanggal         time.Time `gorm:"type:date" json:"tanggal"`
	BahanID         uint      `gorm:"not null" json:"bahan_id"`
	Bahan           Bahan     `gorm:"foreignKey:BahanID" json:"bahan"`
	Qty             float64   `gorm:"not null" json:"qty"`
	HargaBeliSatuan float64   `gorm:"not null" json:"harga_beli_satuan"` // Histori harga pada hari H
	TotalBiaya      float64   `gorm:"not null" json:"total_biaya"`
	Keterangan      string    `json:"keterangan"`
}

// 8. MASTER RESEP
type Resep struct {
	ID            uint           `gorm:"primaryKey"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
	NamaResep     string         `gorm:"not null" json:"nama_resep"`
	TargetGramasi float64        `gorm:"not null" json:"target_gramasi"` // Total adonan matang dr 1 Resep
	BahanDetail   []ResepBahan   `gorm:"foreignKey:ResepID" json:"bahan_detail"`
}

// KOMPOSISI RESEP (Resep - Bahan)
type ResepBahan struct {
	ID        uint    `gorm:"primaryKey"`
	ResepID   uint    `gorm:"not null" json:"resep_id"`
	BahanID   uint    `gorm:"not null" json:"bahan_id"`
	Bahan     Bahan   `gorm:"foreignKey:BahanID" json:"bahan"`
	Kebutuhan float64 `gorm:"not null" json:"kebutuhan"` // Butuh berapa gr/ml/pcs
}

// 9. RIWAYAT PRODUKSI MASAK (PENGURANGAN BAHAN MENTAH)
type ProduksiMasak struct {
	ID          uint      `gorm:"primaryKey"`
	Tanggal     time.Time `gorm:"type:date" json:"tanggal"`
	ResepID     uint      `gorm:"not null" json:"resep_id"`
	Resep       Resep     `gorm:"foreignKey:ResepID" json:"resep"`
	JumlahBatch float64   `gorm:"not null" json:"jumlah_batch"` // Berapa resep dimasak (misal: 2.5)
	TotalAdonan float64   `gorm:"not null" json:"total_adonan"` // JumlahBatch * TargetGramasi (Prediksi Sistem)
}

// 10. RIWAYAT MATANG (KENYATAAN FISIK ROTI)
type ProduksiMatang struct {
	ID        uint      `gorm:"primaryKey"`
	Tanggal   time.Time `gorm:"type:date" json:"tanggal"`
	BarangID  uint      `gorm:"not null" json:"barang_id"`
	Barang    Barang    `gorm:"foreignKey:BarangID" json:"barang"`
	QtyMatang int       `gorm:"not null" json:"qty_matang"` // Fisik utuh siap jual
}

// 11. SISA LAYAK JUAL (CARRY-OVER STOCK)
type SisaLayakJual struct {
	ID       uint      `gorm:"primaryKey"`
	Tanggal  time.Time `gorm:"type:date" json:"tanggal"` // Sisa yang diakui di akhir hari ini
	BarangID uint      `gorm:"not null" json:"barang_id"`
	Barang   Barang    `gorm:"foreignKey:BarangID" json:"barang"`
	QtySisa  int       `gorm:"not null" json:"qty_sisa"`
}

// 12. JURNAL EFISIENSI RESEP (SELISIH MISTERIUS / WASTE DAPUR)
type JurnalEfisiensi struct {
	ID           uint      `gorm:"primaryKey"`
	Tanggal      time.Time `gorm:"type:date" json:"tanggal"`
	ResepID      uint      `gorm:"not null" json:"resep_id"`
	Resep        Resep     `gorm:"foreignKey:ResepID" json:"resep"`
	ModalAdonan  float64   `json:"modal_adonan"`  // Prediksi (gr)
	HasilRoti    float64   `json:"hasil_roti"`    // Kenyataan dikonversi ke gr
	SelisihWaste float64   `json:"selisih_waste"` // Minus (buang) atau Plus (mekar)
	Kinerja      float64   `json:"kinerja"`       // (HasilRoti / ModalAdonan) * 100
}

// 13. STOCK OPNAME (SIDAK GUDANG BAHAN FISIK)
type StockOpname struct {
	ID         uint      `gorm:"primaryKey"`
	Tanggal    time.Time `json:"tanggal"`
	BahanID    uint      `gorm:"not null" json:"bahan_id"`
	Bahan      Bahan     `gorm:"foreignKey:BahanID" json:"bahan"`
	StokSistem float64   `json:"stok_sistem"` // Stok di komputer sebelum sidak
	StokFisik  float64   `json:"stok_fisik"`  // Input nyata dari timbangan gudang
	Selisih    float64   `json:"selisih"`     // Fisik - Sistem
	Keterangan string    `json:"keterangan"`
}
