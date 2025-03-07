name: {{.Name}}
on: [push, pull_request]
concurrency:
  group: format('{0}-{1}', ${{"{{"}} github.ref {{"}}"}}, '{{.Name}}')
  cancel-in-progress: true

permissions: read-all

env:
  LAUNCHABLE_ORGANIZATION: "vitess"
  LAUNCHABLE_WORKSPACE: "vitess-app"
  GITHUB_PR_HEAD_SHA: "${{`{{ github.event.pull_request.head.sha }}`}}"

jobs:
  build:
    name: Run endtoend tests on {{.Name}}
    runs-on: {{if .Cores16}}gh-hosted-runners-16cores-1{{else}}gh-hosted-runners-4cores-1{{end}}

    steps:
    - name: Skip CI
      run: |
        if [[ "{{"${{contains( github.event.pull_request.labels.*.name, 'Skip CI')}}"}}" == "true" ]]; then
          echo "skipping CI due to the 'Skip CI' label"
          exit 1
        fi

    - name: Check if workflow needs to be skipped
      id: skip-workflow
      run: |
        skip='false'
        if [[ "{{"${{github.event.pull_request}}"}}" ==  "" ]] && [[ "{{"${{github.ref}}"}}" != "refs/heads/main" ]] && [[ ! "{{"${{github.ref}}"}}" =~ ^refs/heads/release-[0-9]+\.[0-9]$ ]] && [[ ! "{{"${{github.ref}}"}}" =~ "refs/tags/.*" ]]; then
          skip='true'
        fi
        echo Skip ${skip}
        echo "skip-workflow=${skip}" >> $GITHUB_OUTPUT

        PR_DATA=$(curl \
          -H "{{"Authorization: token ${{ secrets.GITHUB_TOKEN }}"}}" \
          -H "Accept: application/vnd.github.v3+json" \
          "{{"https://api.github.com/repos/${{ github.repository }}/pulls/${{ github.event.pull_request.number }}"}}")
        draft=$(echo "$PR_DATA" | jq .draft -r)
        echo "is_draft=${draft}" >> $GITHUB_OUTPUT

    - name: Check out code
      if: steps.skip-workflow.outputs.skip-workflow == 'false'
      uses: actions/checkout@v3

    - name: Check for changes in relevant files
      if: steps.skip-workflow.outputs.skip-workflow == 'false'
      uses: frouioui/paths-filter@main
      id: changes
      with:
        token: ''
        filters: |
          end_to_end:
            - 'go/**/*.go'
            - 'test.go'
            - 'Makefile'
            - 'build.env'
            - 'go.sum'
            - 'go.mod'
            - 'proto/*.proto'
            - 'tools/**'
            - 'config/**'
            - 'bootstrap.sh'
            - '.github/workflows/{{.FileName}}'
            {{- if or (contains .Name "onlineddl") (contains .Name "schemadiff") }}
            - 'go/test/endtoend/onlineddl/vrepl_suite/testdata'
            {{- end}}

    - name: Set up Go
      if: steps.skip-workflow.outputs.skip-workflow == 'false' && steps.changes.outputs.end_to_end == 'true'
      uses: actions/setup-go@v4
      with:
        go-version: 1.21.6

    - name: Set up python
      if: steps.skip-workflow.outputs.skip-workflow == 'false' && steps.changes.outputs.end_to_end == 'true'
      uses: actions/setup-python@v4

    - name: Tune the OS
      if: steps.skip-workflow.outputs.skip-workflow == 'false' && steps.changes.outputs.end_to_end == 'true'
      run: |
        # Limit local port range to not use ports that overlap with server side
        # ports that we listen on.
        sudo sysctl -w net.ipv4.ip_local_port_range="22768 65535"
        # Increase the asynchronous non-blocking I/O. More information at https://dev.mysql.com/doc/refman/5.7/en/innodb-parameters.html#sysvar_innodb_use_native_aio
        echo "fs.aio-max-nr = 1048576" | sudo tee -a /etc/sysctl.conf
        sudo sysctl -p /etc/sysctl.conf

    - name: Get dependencies
      if: steps.skip-workflow.outputs.skip-workflow == 'false' && steps.changes.outputs.end_to_end == 'true'
      run: |
        {{if .InstallXtraBackup}}

        # Setup Percona Server for MySQL 8.0
        sudo apt-get update
        sudo apt-get install -y lsb-release gnupg2 curl
        wget https://repo.percona.com/apt/percona-release_latest.$(lsb_release -sc)_all.deb
        sudo DEBIAN_FRONTEND="noninteractive" dpkg -i percona-release_latest.$(lsb_release -sc)_all.deb
        sudo percona-release setup ps80
        sudo apt-get update

        # Install everything else we need, and configure
        sudo apt-get install -y percona-server-server percona-server-client make unzip g++ etcd git wget eatmydata xz-utils libncurses5

        {{else}}

        # Get key to latest MySQL repo
        sudo apt-key adv --keyserver keyserver.ubuntu.com --recv-keys A8D3785C
        # Setup MySQL 8.0
        wget -c https://dev.mysql.com/get/mysql-apt-config_0.8.29-1_all.deb
        echo mysql-apt-config mysql-apt-config/select-server select mysql-8.0 | sudo debconf-set-selections
        sudo DEBIAN_FRONTEND="noninteractive" dpkg -i mysql-apt-config*
        sudo apt-get update
        # Install everything else we need, and configure
        sudo apt-get install -y mysql-server mysql-client make unzip g++ etcd curl git wget eatmydata xz-utils libncurses5

        {{end}}

        sudo service mysql stop
        sudo service etcd stop
        sudo ln -s /etc/apparmor.d/usr.sbin.mysqld /etc/apparmor.d/disable/
        sudo apparmor_parser -R /etc/apparmor.d/usr.sbin.mysqld
        go mod download

        # install JUnit report formatter
        go install github.com/vitessio/go-junit-report@HEAD

        {{if .InstallXtraBackup}}

        sudo apt-get install -y percona-xtrabackup-80 lz4

        {{end}}

    {{if .MakeTools}}

    - name: Installing zookeeper and consul
      if: steps.skip-workflow.outputs.skip-workflow == 'false' && steps.changes.outputs.end_to_end == 'true'
      run: |
          make tools

    {{end}}

    - name: Setup launchable dependencies
      if: steps.skip-workflow.outputs.is_draft == 'false' && steps.skip-workflow.outputs.skip-workflow == 'false' && steps.changes.outputs.end_to_end == 'true' && github.base_ref == 'main'
      run: |
        # Get Launchable CLI installed. If you can, make it a part of the builder image to speed things up
        pip3 install --user launchable~=1.0 > /dev/null

        # verify that launchable setup is all correct.
        launchable verify || true

        # Tell Launchable about the build you are producing and testing
        launchable record build --name "$GITHUB_RUN_ID" --no-commit-collection --source .

    - name: Run cluster endtoend test
      if: steps.skip-workflow.outputs.skip-workflow == 'false' && steps.changes.outputs.end_to_end == 'true'
      timeout-minutes: 45
      run: |
        # We set the VTDATAROOT to the /tmp folder to reduce the file path of mysql.sock file
        # which musn't be more than 107 characters long.
        export VTDATAROOT="/tmp/"
        source build.env

        set -exo pipefail

        {{if .LimitResourceUsage}}
        # Increase our open file descriptor limit as we could hit this
        ulimit -n 65536
        cat <<-EOF>>./config/mycnf/mysql80.cnf
        innodb_buffer_pool_dump_at_shutdown=OFF
        innodb_buffer_pool_in_core_file=OFF
        innodb_buffer_pool_load_at_startup=OFF
        innodb_buffer_pool_size=64M
        innodb_doublewrite=OFF
        innodb_flush_log_at_trx_commit=0
        innodb_flush_method=O_DIRECT
        innodb_numa_interleave=ON
        innodb_adaptive_hash_index=OFF
        sync_binlog=0
        sync_relay_log=0
        performance_schema=OFF
        slow-query-log=OFF
        EOF
        {{end}}

        {{if .EnableBinlogTransactionCompression}}
        cat <<-EOF>>./config/mycnf/mysql80.cnf
        binlog-transaction-compression=ON
        EOF
        {{end}}

        # run the tests however you normally do, then produce a JUnit XML file
        eatmydata -- go run test.go -docker={{if .Docker}}true -flavor={{.Platform}}{{else}}false{{end}} -follow -shard {{.Shard}}{{if .PartialKeyspace}} -partial-keyspace=true {{end}} | tee -a output.txt | go-junit-report -set-exit-code > report.xml

    - name: Print test output and Record test result in launchable if PR is not a draft
      if: steps.skip-workflow.outputs.skip-workflow == 'false' && steps.changes.outputs.end_to_end == 'true' && always()
      run: |
        if [[ "{{"${{steps.skip-workflow.outputs.is_draft}}"}}" ==  "false" ]]; then
          # send recorded tests to launchable
          launchable record tests --build "$GITHUB_RUN_ID" go-test . || true
        fi

        # print test output
        cat output.txt
