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

	Status  string `gorm:"default:'KIRIM'"` // 'KIRIM' atau 'SELESAI'
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
