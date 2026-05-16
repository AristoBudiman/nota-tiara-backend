package models

// =====================================================================
// DTO (DATA TRANSFER OBJECT) KHUSUS SWAGGER & PAYLOAD
// File ini murni untuk mendefinisikan bentuk JSON dari frontend.
// =====================================================================

// MessageResponse merepresentasikan format balikan sukses secara generik
type MessageResponse struct {
	Message string `json:"message" example:"Operasi berhasil dilakukan"`
}

// ErrorResponse merepresentasikan balasan error standar API (Gagal)
type ErrorResponse struct {
	Error string `json:"error" example:"Terjadi kesalahan pada server"`
}

// =====================================================================
// DTO SISTEM NOTA MASTER
// =====================================================================

// KemasanInput merepresentasikan objek di dalam array kemasan_detail
type KemasanInput struct {
	BahanID   uint    `json:"bahan_id" example:"2"`
	Kebutuhan float64 `json:"kebutuhan" example:"1.5"`
}

// BarangInput merepresentasikan JSON utuh saat Create / Update Barang
type BarangInput struct {
	NamaBarang      string         `json:"NamaBarang" example:"Roti Tawar Spesial"`
	HargaDefault    float64        `json:"HargaDefault" example:"15000"`
	ResepID         *uint          `json:"resep_id" example:"1"`
	MetodeKonversi  string         `json:"metode_konversi" example:"BAGI"`
	KebutuhanAdonan float64        `json:"kebutuhan_adonan" example:"0.5"`
	MasaSimpan      int            `json:"masa_simpan" example:"3"`
	KemasanDetail   []KemasanInput `json:"kemasan_detail"`
}

// UrutanBarangInput merepresentasikan JSON utuh saat Drag & Drop urutan
type UrutanBarangInput struct {
	ID     uint `json:"id" example:"1"`
	Urutan int  `json:"urutan" example:"2"`
}

// =====================================================================
// DTO SISTEM NOTA KIRIMAN (REGULER & DASHBOARD SALES)
// =====================================================================

// NotaDetailInput merepresentasikan baris item barang di dalam Nota
type NotaDetailInput struct {
	ID          uint    `json:"id" example:"0"` // 0 saat Create baru, isi ID asli saat Update
	BarangID    uint    `json:"barang_id" validate:"required" example:"12"`
	BanyakKirim int     `json:"banyak_kirim" validate:"required" example:"100"` // <-- Disesuaikan
	BanyakRetur int     `json:"banyak_retur" example:"5"`                       // <-- Disesuaikan
	HargaJual   float64 `json:"harga_jual" validate:"required" example:"15000"` // <-- Disesuaikan
}

// NotaInput merepresentasikan payload utama saat Create / Update Nota Reguler
type NotaInput struct {
	NoNota       string            `json:"no_nota" validate:"required" example:"NT/20260427/15-0017"`
	TokoID       uint              `json:"toko_id" validate:"required" example:"5"`
	TanggalKirim string            `json:"tanggal_kirim" validate:"required" example:"2026-04-27"`
	Status       string            `json:"status" validate:"required" enums:"KIRIM,DIBATALKAN,SELESAI" example:"KIRIM"`
	IsLunas      bool              `json:"is_lunas" example:"false"`
	AssignedTo   uint              `json:"assigned_to" example:"2"`
	TotalDiskon  float64           `json:"total_diskon" example:"0"`
	TotalVoucher float64           `json:"total_voucher" example:"50000"`
	Details      []NotaDetailInput `json:"details" validate:"required"`
}

// NextNotaResponse merepresentasikan balikan khusus untuk endpoint generate nomor nota
type NextNotaResponse struct {
	NoNota string `json:"no_nota" example:"NT/20260427/15-0017"`
}

// DashboardSalesResponse merepresentasikan rangkuman layar kunjungan Sales di lapangan
type DashboardSalesResponse struct {
	Aktif   []Nota        `json:"aktif"`
	Tugas   []Nota        `json:"tugas"`
	TugasPO []NotaPesanan `json:"tugas_po"`
}

// =====================================================================
// DTO SISTEM NOTA PESANAN (PO)
// =====================================================================

// NotaPesananKemasanInput merepresentasikan bahan kemasan untuk barang kustom PO
type NotaPesananKemasanInput struct {
	BahanID   uint    `json:"bahan_id" validate:"required" example:"2"`
	Kebutuhan float64 `json:"kebutuhan" validate:"required" example:"1.5"`
}

// NotaPesananDetailInput merepresentasikan item pesanan PO
type NotaPesananDetailInput struct {
	BarangID        *uint                     `json:"barang_id" example:"12"` // Bisa null jika barang custom
	NamaBarangBebas string                    `json:"nama_barang_bebas" validate:"required" example:"Bolu Karamel Custom"`
	Banyak          int                       `json:"banyak" validate:"required" example:"50"`
	HargaJual       float64                   `json:"harga_jual" validate:"required" example:"35000"`
	ResepID         *uint                     `json:"resep_id" example:"3"`
	Gramasi         float64                   `json:"gramasi" example:"500"`
	KemasanDetail   []NotaPesananKemasanInput `json:"kemasan_detail"`
}

// NotaPesananInput merepresentasikan payload utama saat Create / Update PO
type NotaPesananInput struct {
	NoNota           string                   `json:"no_nota" validate:"required" example:"PO/20260430/15-0001"`
	NamaPemesan      string                   `json:"nama_pemesan" validate:"required" example:"Ibu Rina"`
	TanggalKirim     string                   `json:"tanggal_kirim" validate:"required" example:"2026-04-30"`
	JenisPengambilan string                   `json:"jenis_pengambilan" validate:"required" enums:"PABRIK,MITRA" example:"MITRA"`
	TokoID           *uint                    `json:"toko_id" example:"15"` // Null jika ambil di pabrik
	AssignedTo       uint                     `json:"assigned_to" example:"2"`
	Status           string                   `json:"status" validate:"required" enums:"MENUNGGU,DIPROSES,DIKIRIM,DIAMBIL,DIBATALKAN" example:"MENUNGGU"`
	IsLunas          bool                     `json:"is_lunas" example:"false"`
	Ongkir           float64                  `json:"ongkir" example:"15000"`
	UangMuka         float64                  `json:"uang_muka" example:"50000"`
	TotalVoucher     float64                  `json:"total_voucher" example:"10000"`
	Details          []NotaPesananDetailInput `json:"details" validate:"required"`
}

// =====================================================================
// DTO LAPORAN & RANGKUMAN
// =====================================================================

// CatatanBesarResponse merepresentasikan baris data pada tabel Catatan Besar
type CatatanBesarResponse struct {
	NamaBarang string  `json:"nama_barang" example:"Roti Tawar"`
	NamaToko   string  `json:"nama_toko" example:"Toko Abadi"`
	Siklus     string  `json:"siklus" example:"HARIAN"`
	IsHarian   bool    `json:"is_harian" example:"true"`
	QtyKirim   int     `json:"qty_kirim" example:"50"`
	QtyRetur   int     `json:"qty_retur" example:"5"`
	HargaKirim float64 `json:"harga_kirim" example:"750000"`
	HargaRetur float64 `json:"harga_retur" example:"75000"`
}

// RangkumanPerTokoResponse merepresentasikan performa laku barang per toko
type RangkumanPerTokoResponse struct {
	NamaBarang string  `json:"nama_barang" example:"Roti Manis"`
	TotalKirim int     `json:"total_kirim" example:"100"`
	TotalRetur int     `json:"total_retur" example:"10"`
	TotalLaku  int     `json:"total_laku" example:"90"`
	Persentase float64 `json:"persentase" example:"10.0"`
}

// CatatanPesananResponse merepresentasikan rekap PO harian per toko
type CatatanPesananResponse struct {
	NamaBarangBebas  string  `json:"nama_barang_bebas" example:"Bolu Karamel"`
	NamaTokoSnapshot string  `json:"nama_toko" example:"PABRIK"`
	JenisPengambilan string  `json:"jenis_pengambilan" example:"PABRIK"`
	TotalBanyak      int     `json:"total_banyak" example:"25"`
	TotalRupiah      float64 `json:"total_rupiah" example:"875000"`
}

// RangkumanPesananResponse merepresentasikan tabulasi omzet bulanan PO
type RangkumanPesananResponse struct {
	TotalPendapatan float64                  `json:"total_pendapatan" example:"5000000"`
	TotalPesanan    int                      `json:"total_pesanan" example:"10"`
	TotalDiskon     float64                  `json:"total_diskon" example:"100000"`
	PerTitik        []map[string]interface{} `json:"per_titik"`
	DetailBarang    []map[string]interface{} `json:"detail_barang"`
}

// =====================================================================
// DTO MASTER INVENTORY (BAHAN & RESEP)
// =====================================================================

// BahanInput merepresentasikan payload saat Create / Update Master Bahan
type BahanInput struct {
	NamaBahan    string  `json:"nama_bahan" validate:"required" example:"Tepung Cakra Kembar"`
	Satuan       string  `json:"satuan" validate:"required" example:"gr"`
	HargaSaatIni float64 `json:"harga_saat_ini" example:"12500"`
	BatasMinimum float64 `json:"batas_minimum" example:"5000"`
	Stok         float64 `json:"stok" example:"15000"`
}

// UrutanBahanInput merepresentasikan payload Drag & Drop Bahan
type UrutanBahanInput struct {
	ID     uint `json:"id" validate:"required" example:"1"`
	Urutan int  `json:"urutan" validate:"required" example:"2"`
}

// ResepBahanInput merepresentasikan bahan penyusun resep
type ResepBahanInput struct {
	BahanID   uint    `json:"bahan_id" validate:"required" example:"2"`
	Kebutuhan float64 `json:"kebutuhan" validate:"required" example:"250.5"`
}

// ResepInput merepresentasikan payload saat Create / Update Master Resep
type ResepInput struct {
	NamaResep     string            `json:"nama_resep" validate:"required" example:"Adonan Roti Manis"`
	TargetGramasi float64           `json:"target_gramasi" validate:"required" example:"2000"`
	BahanDetail   []ResepBahanInput `json:"bahan_detail" validate:"required"`
}

// =====================================================================
// DTO OPERASIONAL INVENTORY (BELI, MASAK, MATANG, OPNAME)
// =====================================================================

// PembelianBahanInput merepresentasikan input catatan belanja bahan baku
type PembelianBahanInput struct {
	Tanggal         string  `json:"tanggal" validate:"required" example:"2026-05-16"`
	BahanID         uint    `json:"bahan_id" validate:"required" example:"3"`
	Qty             float64 `json:"qty" validate:"required" example:"50"`
	HargaBeliSatuan float64 `json:"harga_beli_satuan" validate:"required" example:"15000"`
	Keterangan      string  `json:"keterangan" example:"Beli di Pasar Legi"`
	IsLunas         bool    `json:"is_lunas" example:"true"`
}

// StatusPembelianInput merepresentasikan saklar lunas/hutang
type StatusPembelianInput struct {
	IsLunas bool `json:"is_lunas" validate:"required" example:"true"`
}

// ProduksiMasakInput merepresentasikan catatan masak adonan harian
type ProduksiMasakInput struct {
	Tanggal     string  `json:"tanggal" validate:"required" example:"2026-05-16"`
	ResepID     uint    `json:"resep_id" validate:"required" example:"2"`
	JumlahBatch float64 `json:"jumlah_batch" validate:"required" example:"1.5"`
}

// ProduksiMatangInput merepresentasikan hasil matang oven harian
type ProduksiMatangInput struct {
	Tanggal   string `json:"tanggal" validate:"required" example:"2026-05-16"`
	BarangID  uint   `json:"barang_id" validate:"required" example:"5"`
	QtyMatang int    `json:"qty_matang" validate:"required" example:"150"`
}

// BarangRusakInput merepresentasikan pencatatan afkir/gratisan
type BarangRusakInput struct {
	Tanggal    string `json:"tanggal" validate:"required" example:"2026-05-16"`
	BarangID   uint   `json:"barang_id" validate:"required" example:"5"`
	Qty        int    `json:"qty" validate:"required" example:"5"`
	Keterangan string `json:"keterangan" validate:"required" example:"Gosong di oven / Dikasihkan ke tetangga"`
}

// TutupBukuInput merepresentasikan trigger tanggal tutup buku
type TutupBukuInput struct {
	Tanggal string `json:"tanggal" validate:"required" example:"2026-05-16"`
}

// StockOpnameInput merepresentasikan koreksi stok fisik gudang
type StockOpnameInput struct {
	BahanID    uint    `json:"bahan_id" validate:"required" example:"3"`
	StokFisik  float64 `json:"stok_fisik" validate:"required" example:"4500.5"`
	Keterangan string  `json:"keterangan" example:"Tumpah 500 gram"`
}

// JurnalTutupBukuResponse merepresentasikan struktur laporan tutup buku harian
type JurnalTutupBukuResponse struct {
	Jurnal []map[string]interface{} `json:"jurnal"`
	Sisa   []map[string]interface{} `json:"sisa"`
}

// =====================================================================
// DTO KAS, KEUANGAN & ANALISIS ASET
// =====================================================================

// KasInput merepresentasikan transaksi manual untuk brankas kas
type KasInput struct {
	Tanggal    string  `json:"tanggal" validate:"required" example:"2026-05-16"`
	Kategori   string  `json:"kategori" validate:"required" enums:"REGULER,PESANAN,BAHAN,RUMAH_TANGGA" example:"RUMAH_TANGGA"`
	Jenis      string  `json:"jenis" validate:"required" enums:"MASUK,KELUAR" example:"KELUAR"`
	Nominal    float64 `json:"nominal" validate:"required" example:"150000"`
	Keterangan string  `json:"keterangan" example:"Beli galon dan token listrik"`
	NoNotaRef  string  `json:"no_nota_ref" example:"-"`
}

// PengaturanKasResponse merepresentasikan status saklar sinkronisasi kas
type PengaturanKasResponse struct {
	IsActive bool `json:"is_active" example:"true"`
}

// ToggleKasInput merepresentasikan payload untuk mematikan/menyalakan sinkronisasi kas
type ToggleKasInput struct {
	IsActive bool `json:"is_active" validate:"required" example:"true"`
}

// SnapshotAsetInput merepresentasikan trigger untuk mengunci saldo akhir bulan
type SnapshotAsetInput struct {
	Bulan string `json:"bulan" validate:"required" example:"2026-05-01"`
}

// AnalisisAsetResponse merepresentasikan balikan live dashboard aset
type AnalisisAsetResponse struct {
	Live            map[string]interface{} `json:"live"`
	PriveBulanIni   float64                `json:"prive_bulan_ini" example:"1500000"`
	BulanLalu       map[string]interface{} `json:"bulan_lalu"`
	TanggalAnalisis string                 `json:"tanggal_analisis" example:"2026-05-16"`
	AwalPrive       string                 `json:"awal_prive" example:"2026-05-01"`
}
