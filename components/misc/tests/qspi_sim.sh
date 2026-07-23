#!/bin/bash
#
# qspi_sim.sh - Run all four QSPI testbenches and report pass/fail status.
#
# Usage: qspi_sim.sh
#
# Runs the following testbenches in separate work directories:
#   1. qspi_flash_model_tb     - flash model protocol assertions
#   2. qspi_read_engine_tb     - read engine single/quad modes
#   3. qspi_flash_ctrl_tb      - controller double-buffer + prefetch
#   4. qspi_flash_ctrl_pads_tb - controller through GF180 pad wrapper
#
# Exits 0 if all pass, nonzero if any fails.

REPO_ROOT="$(cd "$(dirname "$0")/../../../" && pwd)"
COMPONENTS_MISC="${REPO_ROOT}/components/misc"
TESTS_DIR="${COMPONENTS_MISC}/tests"
WORKDIR="/tmp/qspi_sim_work"

# Clean and prepare work directory
rm -rf "$WORKDIR"
mkdir -p "$WORKDIR"

# Test list: (tb_entity_name, workdir_suffix)
declare -a TESTS=(
  "qspi_flash_model_tb|model"
  "qspi_read_engine_tb|engine"
  "qspi_flash_ctrl_tb|ctrl"
  "qspi_flash_ctrl_pads_tb|pads"
)

# Common compilation sources (in order)
SOURCES=(
  "${COMPONENTS_MISC}/../cpu/cpu2j0_pkg.vhd"
  "${TESTS_DIR}/qspi_flash_model.vhd"
  "${COMPONENTS_MISC}/qspi_flash_ctrl.vhd"
  "${COMPONENTS_MISC}/gf180_qspi_io.vhd"
)

# Testbench-specific sources
declare -A TB_SOURCES=(
  ["qspi_flash_model_tb"]="${TESTS_DIR}/qspi_flash_model_tb.vhd"
  ["qspi_read_engine_tb"]="${TESTS_DIR}/qspi_read_engine_tb.vhd"
  ["qspi_flash_ctrl_tb"]="${TESTS_DIR}/qspi_flash_ctrl_tb.vhd"
  ["qspi_flash_ctrl_pads_tb"]="${TESTS_DIR}/qspi_flash_ctrl_pads_tb.vhd"
)

PASSED=0
FAILED=0
FAILED_TESTS=""

# Run each testbench
for test_spec in "${TESTS[@]}"; do
  TB_NAME="${test_spec%|*}"
  TB_SUFFIX="${test_spec#*|}"
  TB_WORKDIR="${WORKDIR}/${TB_SUFFIX}"

  mkdir -p "$TB_WORKDIR"

  echo "=== Running ${TB_NAME} ==="

  # Compile common sources + testbench-specific source
  ghdl -a --std=93 --workdir="$TB_WORKDIR" "${SOURCES[@]}" "${TB_SOURCES[${TB_NAME}]}" 2>&1 | grep -v "warning: declaration of" | grep -v "procedure sck_pulse" || true

  # Elaborate
  ghdl -e --std=93 --workdir="$TB_WORKDIR" "${TB_NAME}" 2>&1 || true

  # Run and capture result
  RUN_OUTPUT=$(ghdl -r --std=93 --workdir="$TB_WORKDIR" "${TB_NAME}" 2>&1)
  if echo "$RUN_OUTPUT" | grep -q "PASSED"; then
    echo "PASS: ${TB_NAME}"
    ((PASSED++))
  else
    echo "FAIL: ${TB_NAME}"
    ((FAILED++))
    FAILED_TESTS="${FAILED_TESTS}  - ${TB_NAME}\n"
  fi
  echo ""
done

# Summary
echo "=========================================="
echo "QSPI Simulation Summary"
echo "=========================================="
echo "Passed: ${PASSED}/4"
echo "Failed: ${FAILED}/4"

if [ ${FAILED} -gt 0 ]; then
  echo -e "\nFailed testbenches:"
  echo -e "${FAILED_TESTS}"
  exit 1
fi

exit 0
