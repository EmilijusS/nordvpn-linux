#!/bin/bash
category="${1}"
pattern="${2}"

set -ux

source "${CI_PROJECT_DIR}"/ci/env.sh

if ! "${CI_PROJECT_DIR}"/ci/install_deb.sh; then
    echo "failed to install deb"
    exit 1
fi

echo "~~~Diagnose wireguard for possible problems on gitlab runner"
echo "~~~ ip a"
ip a
echo "~~~~~~~~~~~~~~"
echo "lsmod"
lsmod
echo "~~~~~~~~~~~~~~"
echo "sudo ip link add dev wg0diagnose type wireguard"
sudo ip link add dev wg0diagnose type wireguard
echo "~~~~~~~~~~~~~~"
echo "~~~ ip a"
ip a
echo "~~~~~~~~~~~~~~"
echo "sudo ip link del wg0diagnose"
sudo ip link del wg0diagnose
echo "~~~~~~~~~~~~~~"


mkdir -p "${CI_PROJECT_DIR}"/dist/logs

cd "${CI_PROJECT_DIR}"/test/qa || exit

args=()

case "${category}" in
    "all")
        ;;
    *)
	args+=("test_${category}.py")
        ;;
esac

case "${pattern}" in
    "")
        ;;
    *)
	args+=("-k ${pattern}")
        ;;
esac


mkdir -p "${CI_PROJECT_DIR}"/"${COVERDIR}" 

if ! sudo grep -q "export GOCOVERDIR=${CI_PROJECT_DIR}/${COVERDIR}" "/etc/init.d/nordvpn"; then
    sudo sed -i "1a export GOCOVERDIR=${CI_PROJECT_DIR}/${COVERDIR}" "/etc/init.d/nordvpn"
fi

python3 -m pytest -v --timeout 180 -rsx -x --timeout-method=thread -o log_cli=true "${args[@]}"

if ! sudo grep -q "export GOCOVERDIR=${CI_PROJECT_DIR}/${COVERDIR}" "/etc/init.d/nordvpn"; then
    sudo sed -i "2d" "/etc/init.d/nordvpn"
fi

# # To print goroutine profile when debugging:
# RET=$?
# if [ $RET != 0 ]; then
#     curl http://localhost:6960/debug/pprof/goroutine?debug=1
# fi
# exit $RET
