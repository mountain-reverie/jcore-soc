library ieee;
use ieee.std_logic_1164.all;
use work.cpu2j0_pack.all;
use work.data_bus_pack.all;

-- Splits the 0xABCDxxxx peripheral bus (DEV_PERIPH) to three slaves by
-- sub-address: UART @0x100 (a(8)='1'), AIC @0x040 (a(6)='1'), else GPIO @0x000.
-- One slave sees a live request at a time; the others see an idle (en='0')
-- request; the master sees the selected slave's response.
entity periph_mux is
  port (
    cpu_o  : in  cpu_data_o_t;   -- from cpu0_periph_dbus_o
    cpu_i  : out cpu_data_i_t;   -- to   cpu0_periph_dbus_i
    uart_o : out cpu_data_o_t; uart_i : in cpu_data_i_t;
    aic_o  : out cpu_data_o_t; aic_i  : in cpu_data_i_t;
    gpio_o : out cpu_data_o_t; gpio_i : in cpu_data_i_t);
end entity;

architecture rtl of periph_mux is
begin
  -- Copy the whole request record to every slave, then gate each slave's `en`
  -- so exactly one is active; select that slave's response back to the master.
  -- (Record-copy + sub-element override is well-defined in a process: the later
  -- assignment to `.en` wins. Copying the whole record avoids enumerating every
  -- field of cpu_data_o_t.)
  process(cpu_o, uart_i, aic_i, gpio_i)
  begin
    uart_o <= cpu_o; uart_o.en <= '0';
    aic_o  <= cpu_o; aic_o.en  <= '0';
    gpio_o <= cpu_o; gpio_o.en <= '0';
    if cpu_o.a(8) = '1' then          -- 0x100 -> UART
      uart_o.en <= cpu_o.en;
      cpu_i <= uart_i;
    elsif cpu_o.a(6) = '1' then       -- 0x040 -> AIC
      aic_o.en <= cpu_o.en;
      cpu_i <= aic_i;
    else                              -- 0x000 -> GPIO
      gpio_o.en <= cpu_o.en;
      cpu_i <= gpio_i;
    end if;
  end process;
end architecture;
