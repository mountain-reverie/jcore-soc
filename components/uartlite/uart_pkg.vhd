-- A simple UART
-- Register compatible with the Xilinx uartlite
-- Only does 8N1

library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;

package uart_pack is

type uart_reg_addr_t is ( DATA, CTRL );

type uart_i_t is record
   dc  : uart_reg_addr_t;
   d   : std_logic_vector(7 downto 0);
   en  : std_logic;
   we  : std_logic;
end record;

type uart_o_t is record
   d   : std_logic_vector(7 downto 0);
   ack : std_logic;
   int : std_logic;
end record;

constant UART_RX_FIFO_LEN : integer := 32;
constant UART_TX_FIFO_LEN : integer := 16;
constant UART_RX_INT_TIMEOUT       : integer := 128;

-- FIFO storage array types are UNCONSTRAINED so that the actual depth is
-- fixed by the entity generics (see uart.vhm). The UART_*_FIFO_LEN constants
-- above remain the DEFAULT depths.
type uart_tx_fifo_t is array (natural range <>) of std_logic_vector(7 downto 0);
type uart_tx_fifo_w_t is record
   -- FIFO index: bound to the max TX depth so pointer arithmetic synthesizes at
   -- the FIFO's real width (a few bits) instead of a full 32-bit `natural`.
   a   : integer range 0 to UART_TX_FIFO_LEN-1;
   d   : std_logic_vector(7 downto 0);
   we  : std_logic;
end record;
type uart_tx_fifo_p_t is record
   wa  : integer range 0 to UART_TX_FIFO_LEN-1;
   ra  : integer range 0 to UART_TX_FIFO_LEN-1;
end record;

type uart_rx_fifo_t is array (natural range <>) of std_logic_vector(7 downto 0);
type uart_rx_fifo_w_t is record
   -- FIFO index: bound to the max RX depth (see uart_tx_fifo_w_t) so pointer
   -- arithmetic synthesizes at the FIFO's real width, not a 32-bit `natural`.
   a   : integer range 0 to UART_RX_FIFO_LEN-1;
   d   : std_logic_vector(7 downto 0);
   we  : std_logic;
end record;
type uart_rx_fifo_p_t is record
   wa  : integer range 0 to UART_RX_FIFO_LEN-1;
   ra  : integer range 0 to UART_RX_FIFO_LEN-1;
end record;

type uart_state_t is ( IDLE, START, DATA, STOP );

type uart_engine_t is record
   s   : uart_state_t;
   sr  : std_logic_vector(7 downto 0);
   b   : integer range 0 to 7;
   phs : integer range 0 to 15;
   full: std_logic;
   ovr : std_logic;
   ferr: std_logic;
   a   : std_logic_vector(1 downto 0);
   m   : integer range 0 to 3;
end record;

constant UART_ENGINE_RESET : uart_engine_t := ( IDLE, (others => '0'), 0, 15, '0', '0', '0', (others => '0'), 0 );

type uart_reg_t is record
   rx  : uart_engine_t;
   rxp : uart_rx_fifo_p_t;
   tx  : uart_engine_t;
   txp : uart_tx_fifo_p_t;
   dds : unsigned(12 downto 0);
   itimeout   : integer range 0 to UART_RX_INT_TIMEOUT-1; -- interrupt timer flag
   rxint : std_logic;                                -- RX interrupt flag prep
   ien : std_logic;
   txo : std_logic;
   en  : std_logic;
   y   : uart_o_t;
end record;

constant UART_REG_RESET : uart_reg_t := ( UART_ENGINE_RESET, (0,0), UART_ENGINE_RESET, (0,0), (others => '0'), 0 , '1', '0', '1', '0', ( (others => '0'), '0', '0') );

component uartlite generic (
   intcfg : integer := 1;
   fclk   : real := 31.25e6;
   bps    : real := 115.2e3;
   rx_fifo_len : integer := UART_RX_FIFO_LEN;
   tx_fifo_len : integer := UART_TX_FIFO_LEN;
   -- When false, the RX FIFO/shift-register/baud state machine and the
   -- RX/timeout interrupt logic are not synthesized: the rx port is
   -- unused, RX register reads return 0, and no RX-sourced interrupt is
   -- ever asserted. TX path and the bus/register interface are unchanged
   -- regardless of this generic. Default true preserves existing behavior.
   rx_enable : boolean := true);
                   port (
   rst    : in  std_logic;
   clk    : in  std_logic;
   a      : in  uart_i_t;
   y      : out uart_o_t;
   -- The actual serial signals
   rx     : in  std_logic;
   tx     : out std_logic);
end component;

end package;
