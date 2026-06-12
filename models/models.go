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
	SiklusDua         bool `gorm:"not null;default:false"`
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

	Kemasan  []BarangKemasan  `gorm:"foreignKey:BarangID" json:"kemasan_detail"`
	Komposit []BarangKomposit `gorm:"foreignKey:BarangID" json:"komposit_detail"`
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
	JumlahKirim  float64 `gorm:"default:0"`                      // Total harga kirim (Semua barang)
	JumlahRetur  float64 `gorm:"default:0"`                      // Total harga retur (Semua barang)
	TotalDiskon  float64 `gorm:"default:0" json:"total_diskon"`  // <--- BARU
	TotalVoucher float64 `gorm:"default:0" json:"total_voucher"` // <--- BARU
	TotalBayar   float64 `gorm:"default:0"`                      // JumlahKirim - JumlahRetur

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

type Role struct {
	ID          uint         `gorm:"primaryKey" json:"id"`
	NamaRole    string       `gorm:"unique;not null" json:"nama_role"`
	Deskripsi   string       `json:"deskripsi"`
	Permissions []Permission `gorm:"many2many:role_permissions;" json:"permissions"`
}

type Permission struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	Kode      string `gorm:"unique;not null" json:"kode"`
	NamaIzin  string `json:"nama_izin"`
}

type Admin struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	Username string `gorm:"unique;not null" json:"username"`
	Password string `json:"-"`
	Email    string `gorm:"unique" json:"email"`

	// Security & Honeypot fields
	FailedLoginAttempts int        `gorm:"default:0" json:"-"`
	LockedUntil         *time.Time `json:"-"`

	// Legacy column untuk keperluan auto-migrate
	LegacyRole string `gorm:"column:role" json:"-"`

	RoleID uint `gorm:"default:1" json:"role_id"`
	Role   Role `gorm:"foreignKey:RoleID" json:"role"`
}

type RekapToko struct {
	ID         uint    `json:"id"`
	Nama       string  `json:"nama"`
	Kirim      float64 `json:"kirim"`
	Retur      float64 `json:"retur"`
	Diskon     float64 `json:"diskon"`
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
	Diskon     float64       `json:"diskon"`
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

	TotalBayar   float64 `gorm:"default:0"`
	Ongkir       float64 `json:"ongkir"`
	UangMuka     float64 `gorm:"default:0" json:"uang_muka"`     // <--- BARU (DP)
	TotalVoucher float64 `gorm:"default:0" json:"total_voucher"` // <--- BARU
	SisaTagihan  float64 `gorm:"default:0" json:"sisa_tagihan"`
	Status       string  `gorm:"default:'BELUM DIAMBIL'"`
	IsLunas      bool    `gorm:"default:false" json:"is_lunas"` // 'BELUM DIAMBIL' atau 'LUNAS/DIAMBIL'

	AssignedTo uint      `json:"assigned_to"`
	CreatedBy  uint      `json:"created_by"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`

	Details []NotaPesananDetail `gorm:"foreignKey:NotaPesananID"`
}

type NotaPesananDetailKemasan struct {
	ID                  uint    `gorm:"primaryKey"`
	NotaPesananDetailID uint    `gorm:"index" json:"nota_pesanan_detail_id"`
	BahanID             uint    `gorm:"not null" json:"bahan_id"`
	Bahan               Bahan   `gorm:"foreignKey:BahanID" json:"bahan"`
	Kebutuhan           float64 `gorm:"not null" json:"kebutuhan"` // Butuh berapa pcs per 1 roti kustom
}

type NotaPesananDetailKomposit struct {
	ID                  uint          `gorm:"primaryKey" json:"id"`
	NotaPesananDetailID uint          `gorm:"index" json:"nota_pesanan_detail_id"`
	ResepKompositID     uint          `gorm:"not null" json:"resep_komposit_id"`
	ResepKomposit       ResepKomposit `gorm:"foreignKey:ResepKompositID" json:"resep_komposit"`
	Kebutuhan           float64       `gorm:"not null" json:"kebutuhan"` // Total gramasi komposit per 1 pcs barang kustom
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

	KemasanDetail      []NotaPesananDetailKemasan `gorm:"foreignKey:NotaPesananDetailID" json:"kemasan_detail"`
	IsKemasanTerpotong bool                       `gorm:"default:false" json:"is_kemasan_terpotong"`

	KompositDetail      []NotaPesananDetailKomposit `gorm:"foreignKey:NotaPesananDetailID" json:"komposit_detail"`
	IsKompositTerpotong bool                        `gorm:"default:false" json:"is_komposit_terpotong"`
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
	Stok           float64          `gorm:"default:0" json:"stok"`
	HargaSaatIni   float64          `gorm:"default:0" json:"harga_saat_ini"` // Update otomatis dari pembelian terakhir
	BatasMinimum   float64          `gorm:"default:0" json:"batas_minimum"`
	Urutan         int              `gorm:"default:0" json:"urutan"`
	KonversiSatuan []KonversiSatuan `gorm:"foreignKey:BahanID" json:"konversi"`
}

type KonversiSatuan struct {
	ID            uint    `gorm:"primaryKey" json:"id"`
	BahanID       uint    `gorm:"not null" json:"bahan_id"`
	NamaSatuan    string  `gorm:"not null" json:"nama_satuan"`    // Contoh: "Sak", "Dus", "Kg"
	NilaiKonversi float64 `gorm:"not null" json:"nilai_konversi"` // Pengali ke satuan dasar (misal 25000 jika dasarnya gr)
}

type BarangKemasan struct {
	ID        uint    `gorm:"primaryKey"`
	BarangID  uint    `gorm:"not null" json:"barang_id"`
	BahanID   uint    `gorm:"not null" json:"bahan_id"`
	Bahan     Bahan   `gorm:"foreignKey:BahanID" json:"bahan"`
	Kebutuhan float64 `gorm:"not null" json:"kebutuhan"` // (Bisa pecahan, misal 0.25)
}

// 7. RIWAYAT BELANJA (PEMBELIAN BAHAN)
// type PembelianBahan struct {
// 	ID              uint      `gorm:"primaryKey"`
// 	Tanggal         time.Time `gorm:"type:date" json:"tanggal"`
// 	BahanID         uint      `gorm:"not null" json:"bahan_id"`
// 	Bahan           Bahan     `gorm:"foreignKey:BahanID" json:"bahan"`
// 	Qty             float64   `gorm:"not null" json:"qty"`
// 	HargaBeliSatuan float64   `gorm:"not null" json:"harga_beli_satuan"` // Histori harga pada hari H
// 	TotalBiaya      float64   `gorm:"not null" json:"total_biaya"`
// 	Keterangan      string    `json:"keterangan"`
// 	IsLunas         bool      `json:"is_lunas"`
// }

// 7. RIWAYAT BELANJA GABUNGAN (NOTA PEMBELIAN)
type NotaPembelian struct {
	ID         uint           `gorm:"primaryKey"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
	Tanggal    time.Time      `gorm:"type:date" json:"tanggal"`
	TotalBiaya float64        `gorm:"not null" json:"total_biaya"` // Grand Total 1 Struk
	Keterangan string         `json:"keterangan"`
	IsLunas    bool           `json:"is_lunas"`

	Details []NotaPembelianDetail `gorm:"foreignKey:NotaPembelianID" json:"details"`
}

// 7.1. RINCIAN BARANG YANG DIBELI DALAM SATU NOTA
type NotaPembelianDetail struct {
	ID              uint           `gorm:"primaryKey"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
	NotaPembelianID uint           `gorm:"not null" json:"nota_pembelian_id"`
	BahanID         uint           `gorm:"not null" json:"bahan_id"`
	Bahan           Bahan          `gorm:"foreignKey:BahanID" json:"bahan"`
	Qty             float64        `gorm:"not null" json:"qty"`
	HargaBeliSatuan float64        `gorm:"not null" json:"harga_beli_satuan"`
	Subtotal        float64        `gorm:"not null" json:"subtotal"`
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
	QtyMatang float64   `gorm:"not null" json:"qty_matang"` // Fisik utuh siap jual
}

// 11. SISA LAYAK JUAL (CARRY-OVER STOCK)
type SisaLayakJual struct {
	ID       uint      `gorm:"primaryKey"`
	Tanggal  time.Time `gorm:"type:date" json:"tanggal"` // Sisa yang diakui di akhir hari ini
	BarangID uint      `gorm:"not null" json:"barang_id"`
	Barang   Barang    `gorm:"foreignKey:BarangID" json:"barang"`
	QtySisa  float64   `gorm:"not null" json:"qty_sisa"`
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

// 14. BARANG RUSAK / AFKIR / GRATIS
type BarangRusak struct {
	ID         uint      `gorm:"primaryKey"`
	Tanggal    time.Time `gorm:"type:date" json:"tanggal"`
	BarangID   uint      `gorm:"not null" json:"barang_id"`
	Barang     Barang    `gorm:"foreignKey:BarangID" json:"barang"`
	Qty        float64   `gorm:"not null" json:"qty"`
	Keterangan string    `json:"keterangan"` // Contoh: "Dimakan Tikus", "Tester", "Basi"
}

type TransaksiKas struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Tanggal    time.Time `json:"tanggal"`
	Kategori   string    `json:"kategori"`
	Jenis      string    `json:"jenis"`
	Nominal    float64   `json:"nominal"`
	Keterangan string    `json:"keterangan"`
	NoNotaRef  string    `json:"no_nota_ref"`
	CreatedBy  uint      `json:"created_by"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type PengaturanSistem struct {
	ID    uint   `gorm:"primaryKey"`
	Key   string `gorm:"unique;not null"` // Contoh: "ENABLE_KAS_SYNC"
	Value string `gorm:"not null"`        // Contoh: "true" atau "false"
}

type MasterKas struct {
	ID    uint    `gorm:"primaryKey"`
	Saldo float64 `gorm:"not null;default:0"`
}

type AsetSnapshot struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	Bulan           time.Time `gorm:"type:date;unique;not null" json:"bulan"` // Contoh: 2026-05-01
	TotalKas        float64   `json:"total_kas"`
	TotalPiutang    float64   `json:"total_piutang"`
	TotalPersediaan float64   `json:"total_persediaan"`
	TotalHutang     float64   `json:"total_hutang"`
	AsetBersih      float64   `json:"aset_bersih"`
	CreatedAt       time.Time `json:"created_at"`
}

// 15. MASTER RESEP KOMPOSIT (Sub-Assembly / Pre-Mix)
type ResepKomposit struct {
	ID           uint                  `gorm:"primaryKey" json:"id"`
	DeletedAt    gorm.DeletedAt        `gorm:"index" json:"-"`
	NamaKomposit string                `gorm:"not null" json:"nama_komposit"`
	Details      []ResepKompositDetail `gorm:"foreignKey:ResepKompositID" json:"details"`
}

type ResepKompositDetail struct {
	ID              uint    `gorm:"primaryKey" json:"id"`
	ResepKompositID uint    `gorm:"not null" json:"resep_komposit_id"`
	BahanID         uint    `gorm:"not null" json:"bahan_id"`
	Bahan           Bahan   `gorm:"foreignKey:BahanID" json:"bahan"`
	Rasio           float64 `gorm:"not null" json:"rasio"` // Misal: 4, 2, 7
}

// JEMBATAN MANY-TO-MANY (Satu Barang bisa pakai banyak komposit)
type BarangKomposit struct {
	ID              uint          `gorm:"primaryKey" json:"id"`
	BarangID        uint          `gorm:"not null" json:"barang_id"`
	ResepKompositID uint          `gorm:"not null" json:"resep_komposit_id"`
	ResepKomposit   ResepKomposit `gorm:"foreignKey:ResepKompositID" json:"resep_komposit"`
	Kebutuhan       float64       `gorm:"not null" json:"kebutuhan"` // Total gramasi komposit per 1 pcs barang
}

// 16. RIWAYAT KONVERSI BARANAG (PECAH BARANG)
type KonversiBahan struct {
	ID          uint                  `gorm:"primaryKey"`
	DeletedAt   gorm.DeletedAt        `gorm:"index" json:"-"`
	Tanggal     time.Time             `gorm:"type:date" json:"tanggal"`
	BahanAsalID uint                  `gorm:"not null" json:"bahan_asal_id"`
	BahanAsal   Bahan                 `gorm:"foreignKey:BahanAsalID" json:"bahan_asal"`
	QtyAsal     float64               `gorm:"not null" json:"qty_asal"`
	Keterangan  string                `json:"keterangan"`
	Details     []KonversiBahanDetail `gorm:"foreignKey:KonversiBahanID" json:"details"`
}

type KonversiBahanDetail struct {
	ID              uint    `gorm:"primaryKey"`
	KonversiBahanID uint    `gorm:"not null" json:"konversi_bahan_id"`
	BahanHasilID    uint    `gorm:"not null" json:"bahan_hasil_id"`
	BahanHasil      Bahan   `gorm:"foreignKey:BahanHasilID" json:"bahan_hasil"`
	QtyHasil        float64 `gorm:"not null" json:"qty_hasil"`
}

