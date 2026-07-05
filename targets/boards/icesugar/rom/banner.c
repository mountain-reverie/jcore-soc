/* J1 on iCESugar (iCE40 UP5K), EBR-only: emit a UART banner over uart0 and
   blink the RGB LED via gpio2 d_o(0). No SDRAM, no interrupts -- everything
   runs from on-chip inferred EBR. Kept tiny to fit the up5k EBR budget. */

/* uartlitedb @ 0xABCD0100: a(3)=0 selects data, a(3)=1 selects status.
   (Matches the ULX3S banner: byte stores would land in d(31:24) on the
   big-endian SH-2, so all UART writes are 32-bit.) */
#define UART_DATA   (*(volatile unsigned int *)0xABCD0100u)  /* a(3)=0 */
#define UART_STATUS (*(volatile unsigned int *)0xABCD0108u)  /* a(3)=1 */
#define TX_FULL     (1u << 3)

/* gpio2 @ 0xABCD0000: d_o -> LEDs (jcore,gpio2). */
#define GPIO_BASE   0xABCD0000u
#define GPIO_DATA   (*(volatile unsigned int *)(GPIO_BASE + 0x00u)) /* wr d_o */
#define GPIO_TOGGLE (*(volatile unsigned int *)(GPIO_BASE + 0x08u)) /* XOR d_o */

static void putc_uart(char c)
{
	while (UART_STATUS & TX_FULL)
		;
	UART_DATA = (unsigned int)(unsigned char)c;   /* 32-bit store, see above */
}

static void puts_uart(const char *p)
{
	while (*p)
		putc_uart(*p++);
}

/* eth @ 0xABCD1000 (DEVICE_ETH_ADDR in board.h): spi2 master driving a
   W5500 (WIZ850io PMOD) over SPI. spi2 register semantics (confirmed
   against components/misc/spi2.vhd):
     ETH_CTRL (+0x0), write: bit0 = cs(0) (idle high=1; write 0 to assert
       CS), bit1 = start_txn (write 1 to begin a byte transfer),
       bit2 = cs(1) (unused, W5500 is cs(0) only).
     ETH_CTRL (+0x0), read: bit0 = cs(0), bit1 = busy (1 while a byte
       transfer is in progress).
     ETH_DATA (+0x4), write: bits[7:0] = byte to shift out next transfer.
     ETH_DATA (+0x4), read: bits[7:0] = byte shifted in during the last
       transfer. */
#define ETH_BASE   0xABCD1000u
#define ETH_CTRL   (*(volatile unsigned int *)(ETH_BASE + 0x0u))
#define ETH_DATA   (*(volatile unsigned int *)(ETH_BASE + 0x4u))
#define ETH_CTRL_CS0    0x1u
#define ETH_CTRL_START  0x2u
#define ETH_CTRL_BUSY   0x2u

static void spi_assert(void)
{
	ETH_CTRL = 0u;                 /* cs(0) = 0: assert CS (idle high) */
}

static void spi_deassert(void)
{
	ETH_CTRL = ETH_CTRL_CS0;       /* cs(0) = 1: deassert CS */
}

/* One SPI byte transfer; CS must already be asserted. */
static unsigned char spi_byte(unsigned char txval)
{
	ETH_DATA = txval;
	ETH_CTRL = ETH_CTRL_START;     /* cs(0)=0, start_txn=1 */
	while (ETH_CTRL & ETH_CTRL_BUSY)
		;
	return (unsigned char)ETH_DATA;
}

/* W5500 common-block (BSB=0) register write, VDM (variable data length)
   mode: 3-byte header (addr_hi, addr_lo, control) then n data bytes, all
   under one CS assertion. control = 0x04 for a common-block write. */
static void w5500_write(unsigned int addr, const unsigned char *buf, int n)
{
	int i;
	spi_assert();
	spi_byte((unsigned char)(addr >> 8));
	spi_byte((unsigned char)(addr & 0xffu));
	spi_byte(0x04u);
	for (i = 0; i < n; i++)
		spi_byte(buf[i]);
	spi_deassert();
}

static void w5500_write8(unsigned int addr, unsigned char val)
{
	w5500_write(addr, &val, 1);
}

/* Reset the W5500 and program MAC/IP/subnet/gateway. Once these are set
   (and MR.PB, the ping-block bit, is left 0) the chip auto-answers ARP and
   ICMP echo entirely in hardware -- no CPU socket code needed. */
static void w5500_init_ping(void)
{
	static const unsigned char shar[6] = { 0x02, 0x00, 0x00, 0x00, 0x00, 0x01 };
	static const unsigned char sipr[4] = { 192, 168, 1, 10 };
	static const unsigned char subr[4] = { 255, 255, 255, 0 };
	static const unsigned char gar[4]  = { 192, 168, 1, 1 };
	volatile unsigned int d;

	w5500_write8(0x0000u, 0x80u);   /* MR.RST: software reset */
	for (d = 0; d < 2000u; d++)
		;                        /* let RST self-clear (datasheet: fast) */

	w5500_write(0x0009u, shar, 6);  /* SHAR: source MAC */
	w5500_write(0x000Fu, sipr, 4);  /* SIPR: source IP */
	w5500_write(0x0005u, subr, 4);  /* SUBR: subnet mask */
	w5500_write(0x0001u, gar, 4);   /* GAR: gateway */
}

/* i2c @ 0xABCD0300 (DEVICE_I2C_ADDR in board.h): 2-bit tristate gpio2 driving
   a DS3231 RTC open-drain over SCL(bit0)/SDA(bit1) (jcore,gpio2 'i2c'
   device; see components/misc/ice_i2c_io.vhd). No hardware I2C master --
   everything below is bit-banged.
     +0x0 value:  write sets drive value d_o (kept 0, see ice_i2c_io);
                  read returns pad level d_i, bits[1:0] = SCL,SDA.
     +0x4 in_out: tristate control d_t; bit=1 -> released/high-Z (pulled
                  high externally), bit=0 -> driven low. */
#define I2C_BASE  0xABCD0300u
#define I2C_VALUE (*(volatile unsigned int *)(I2C_BASE + 0x0u))
#define I2C_TRIS  (*(volatile unsigned int *)(I2C_BASE + 0x4u))
#define I2C_SCL_BIT 0x1u
#define I2C_SDA_BIT 0x2u

static void i2c_delay(void)
{
	volatile int d;
	for (d = 0; d < 8; d++)
		;
}

static void i2c_bus_init(void)
{
	I2C_VALUE = 0u;                     /* d_o kept 0: pure open-drain */
	I2C_TRIS  = I2C_SCL_BIT | I2C_SDA_BIT; /* release both lines (idle high) */
	i2c_delay();
}

static void i2c_scl_high(void) { I2C_TRIS |= I2C_SCL_BIT; i2c_delay(); }
static void i2c_scl_low(void)  { I2C_TRIS &= ~I2C_SCL_BIT; i2c_delay(); }
static void i2c_sda_release(void) { I2C_TRIS |= I2C_SDA_BIT; i2c_delay(); }
static void i2c_sda_low(void)     { I2C_TRIS &= ~I2C_SDA_BIT; i2c_delay(); }
static unsigned int i2c_sda_read(void) { return (I2C_VALUE & I2C_SDA_BIT) ? 1u : 0u; }

/* START: SDA high->low while SCL high. Caller must ensure SCL/SDA are
   already released (idle) before the first START of a transaction. */
static void i2c_start(void)
{
	i2c_sda_release();
	i2c_scl_high();
	i2c_sda_low();
	i2c_scl_low();
}

/* STOP: SDA low->high while SCL high. */
static void i2c_stop(void)
{
	i2c_sda_low();
	i2c_scl_high();
	i2c_sda_release();
	i2c_delay();
}

/* Write one byte MSB-first; returns 1 if the slave ACKed. */
static int i2c_write_byte(unsigned char b)
{
	int i;
	unsigned int ack;
	unsigned int mask = 0x80u;
	for (i = 0; i < 8; i++) {
		if (b & mask)
			i2c_sda_release();
		else
			i2c_sda_low();
		i2c_scl_high();
		i2c_scl_low();
		mask >>= 1;   /* constant shift amount: no runtime shift helper */
	}
	i2c_sda_release();      /* let the slave drive the ACK bit */
	i2c_scl_high();
	ack = (i2c_sda_read() == 0u);   /* ACK = SDA pulled low */
	i2c_scl_low();
	return (int)ack;
}

/* Read one byte MSB-first; sends ACK (more bytes wanted) or NACK (last). */
static unsigned char i2c_read_byte(int ack)
{
	int i;
	unsigned char b = 0u;
	i2c_sda_release();
	for (i = 7; i >= 0; i--) {
		i2c_scl_high();
		b = (unsigned char)((b << 1) | i2c_sda_read());
		i2c_scl_low();
	}
	if (ack)
		i2c_sda_low();
	else
		i2c_sda_release();
	i2c_scl_high();
	i2c_scl_low();
	i2c_sda_release();
	return b;
}

#define DS3231_ADDR 0x68u

/* Write `n` bytes starting at register `reg`. Returns 1 on success (every
   byte, incl. the address+pointer, ACKed). */
static int ds3231_write(unsigned char reg, const unsigned char *buf, int n)
{
	int i, ok = 1;
	i2c_start();
	ok &= i2c_write_byte((unsigned char)(DS3231_ADDR << 1));   /* W */
	ok &= i2c_write_byte(reg);
	for (i = 0; i < n; i++)
		ok &= i2c_write_byte(buf[i]);
	i2c_stop();
	return ok;
}

/* Read `n` bytes starting at register `reg`: write the pointer, repeated
   START, then clock the bytes out (NACK the last one). Returns 1 if the
   address+pointer writes ACKed (read data itself cannot NACK). */
static int ds3231_read(unsigned char reg, unsigned char *buf, int n)
{
	int i, ok = 1;
	i2c_start();
	ok &= i2c_write_byte((unsigned char)(DS3231_ADDR << 1));   /* W */
	ok &= i2c_write_byte(reg);
	i2c_start();                                                /* repeated START */
	ok &= i2c_write_byte((unsigned char)((DS3231_ADDR << 1) | 1u)); /* R */
	for (i = 0; i < n; i++)
		buf[i] = i2c_read_byte(i < n - 1);   /* NACK the last byte */
	i2c_stop();
	return ok;
}

struct ds3231_time {
	unsigned char sec, min, hour, day, date, month, year;   /* all BCD */
};

static unsigned char bin2bcd(unsigned int v)
{
	return (unsigned char)(((v / 10u) << 4) | (v % 10u));
}

static void ds3231_set_time(const struct ds3231_time *t)
{
	unsigned char buf[7];
	buf[0] = t->sec;
	buf[1] = t->min;
	buf[2] = t->hour;
	buf[3] = t->day;
	buf[4] = t->date;
	buf[5] = t->month;
	buf[6] = t->year;
	ds3231_write(0x00u, buf, 7);
}

static void ds3231_read_time(struct ds3231_time *t)
{
	unsigned char buf[7];
	ds3231_read(0x00u, buf, 7);
	t->sec   = buf[0];
	t->min   = buf[1];
	t->hour  = buf[2];
	t->day   = buf[3];
	t->date  = buf[4];
	t->month = buf[5];
	t->year  = buf[6];
}

/* Global, observable read-back time: written once by ds3231_init(), left
   here for inspection (e.g. by a debugger or a future SQW/AIC consumer). */
struct ds3231_time g_rtc_time;

static void puthex4(unsigned int v)
{
	putc_uart("0123456789ABCDEF"[v & 0xFu]);
}

static void puthex8(unsigned char v)
{
	puthex4(v >> 4);
	puthex4(v);
}

/* Program a known time (2024-01-02 03:04:05, BCD), read it back, and print
   a distinct PASS/FAIL line the testbench can look for -- the same pattern
   banner.c already uses for the SPRAM memtest / W5500 programming. */
static void ds3231_init(void)
{
	struct ds3231_time set;
	int match;

	i2c_bus_init();

	set.sec   = bin2bcd(5);
	set.min   = bin2bcd(4);
	set.hour  = bin2bcd(3);
	set.day   = bin2bcd(1);       /* day-of-week: arbitrary, not checked */
	set.date  = bin2bcd(2);
	set.month = bin2bcd(1);
	set.year  = bin2bcd(24);
	ds3231_set_time(&set);

	ds3231_read_time(&g_rtc_time);

	match = (g_rtc_time.sec == set.sec && g_rtc_time.min == set.min &&
	         g_rtc_time.hour == set.hour && g_rtc_time.date == set.date &&
	         g_rtc_time.month == set.month && g_rtc_time.year == set.year);

	puts_uart("DS3231 time=");
	puthex8(g_rtc_time.year); puthex8(g_rtc_time.month);
	puthex8(g_rtc_time.date); puthex8(g_rtc_time.hour);
	puthex8(g_rtc_time.min); puthex8(g_rtc_time.sec);
	puts_uart(match ? " DS3231 PASS\r\n" : " DS3231 FAIL\r\n");
}

#define SPRAM_BASE  0x10000000u
#define SPRAM_WORDS (128u*1024u/4u)   /* 32768 words */

extern unsigned int _spram_load[], _spram_start[], _spram_end[];

/* Runs from SPRAM (proves instruction fetch from SPRAM works). */
static void __attribute__((section(".spram"), noinline))
spram_routine(void)
{
	puts_uart("FROM SPRAM\r\n");
}

/* Write a marching pattern across all 128 KB and read it back. Bounded so the
   sim completes within the testbench window. */
static void spram_memtest(void)
{
	volatile unsigned int *p = (volatile unsigned int *)SPRAM_BASE;
	unsigned int i, bad = 0u;
	for (i = 0u; i < SPRAM_WORDS; i++) p[i] = i * 2654435761u;   /* Knuth hash */
	for (i = 0u; i < SPRAM_WORDS; i++) if (p[i] != i * 2654435761u) bad++;
	puts_uart(bad ? "SPRAM MEMTEST FAIL\r\n" : "SPRAM MEMTEST OK\r\n");
}

void main(void)
{
	puts_uart("J1 on iCESugar: hello\r\n");
	GPIO_DATA = 0x01u;            /* light LED via gpio2 d_o(0) */
	puts_uart("GPIO\r\n");

	/* copy the .spram routine (LMA in EBR) up to SPRAM, then execute it there */
	{
		unsigned int *dst = _spram_start, *src = _spram_load;
		while ((unsigned int)dst < (unsigned int)_spram_end) *dst++ = *src++;
	}
	spram_routine();     /* executes out of SPRAM -> prints "FROM SPRAM" */

	ds3231_init();       /* bit-banged I2C round trip to the DS3231 RTC (early:
	                        keeps its sim assertion ahead of the slow memtest) */

	spram_memtest();     /* proves all 128 KB read/write */

	w5500_init_ping();   /* reset + program MAC/IP/subnet/gateway; the W5500
	                        then auto-answers ARP/ICMP entirely in hardware */
	puts_uart("W5500 INIT OK\r\n");

	/* visible heartbeat; nothing else to poll now the W5500 handles ARP/ICMP
	   on its own once MAC/IP are programmed. */
	{
		unsigned int hb = 0u;
		for (;;) {
			volatile unsigned int d;
			if (++hb >= 400u) {   /* ~0.5 s heartbeat at this spin length */
				hb = 0u;
				GPIO_TOGGLE = 0x01u;
			}
			for (d = 0; d < 1500u; d++)
				;
		}
	}
}
