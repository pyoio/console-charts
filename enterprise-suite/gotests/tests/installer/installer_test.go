package installer

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/lightbend/console-charts/enterprise-suite/gotests/args"
	"github.com/lightbend/console-charts/enterprise-suite/gotests/util"
	"github.com/lightbend/console-charts/enterprise-suite/gotests/util/lbc"

	"github.com/lightbend/console-charts/enterprise-suite/gotests/testenv"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestInstaller(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Installer (lbc.py) Suite")
}

var _ = BeforeSuite(func() {
	testenv.InitEnv()
})

var _ = AfterSuite(func() {
	testenv.CloseEnv()
})

func write(file *os.File, content string) {
	if _, err := file.Write([]byte(content)); err != nil {
		panic(err)
	}
}

var _ = Describe("all:lbc.py", func() {
	var (
		valuesFile *os.File
	)

	BeforeEach(func() {
		var err error
		valuesFile, err = ioutil.TempFile("", "values-*.yaml")
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		err := os.Remove(valuesFile.Name())
		Expect(err).To(Succeed())
	})

	Context("install", func() {
		// Note: this test depends on remote service being up, consider disabling it if it causes problems
		It("should be able to install a remote chart at specified version", func() {
			installer := lbc.DefaultInstaller()
			installer.FailOnWarnings = true
			installer.LocalChart = false
			installer.AdditionalLBCArgs = []string{"--version=1.1"}

			// The test fails currently because version 1.1 doesn't have values-dump.yaml needed to compute helm values
			want := "warning: unable to determine computed helm values - this may lead to incorrect warnings"
			err := installer.Install()
			Expect(err).NotTo(Succeed())
			Expect(err.Error()).To(ContainSubstring(want), "got error: %v", err)
		})
	})

	Context("upgrades", func() {
		Context("disable persistent volumes", func() {
			var installer *lbc.Installer

			BeforeEach(func() {
				preInstaller := lbc.DefaultInstaller()
				preInstaller.UsePersistentVolumes = "true"
				Expect(preInstaller.Install()).To(Succeed(), "install with PVs")

				write(valuesFile, `usePersistentVolumes: false`)
				installer = lbc.DefaultInstaller()
				installer.AdditionalHelmArgs = []string{"-f " + valuesFile.Name()}
			})

			It("should fail if we don't provide --delete-pvcs", func() {
				installer.UsePersistentVolumes = ""
				installer.ForceDeletePVCs = false
				Expect(installer.Install()).ToNot(Succeed())
			})

			It("should succeed if we provide --delete-pvcs", func() {
				installer.UsePersistentVolumes = ""
				installer.ForceDeletePVCs = true
				Expect(installer.Install()).To(Succeed())
			})
		})
	})

	Context("arg parsing", func() {
		It("should fail if conflicting namespaces", func() {
			installer := lbc.DefaultInstaller()
			installer.AdditionalLBCArgs = []string{"--namespace=" + args.ConsoleNamespace}
			installer.AdditionalHelmArgs = []string{"--namespace=my-busted-namespace"}
			Expect(installer.Install()).ToNot(Succeed())
		})
	})

	Context("export yaml", func() {
		It("should be able to export the console yaml for a remote chart", func() {
			installer := lbc.DefaultInstaller()
			installer.AdditionalLBCArgs = []string{"--export-yaml=console", "--version=1.1.0"}
			installer.LocalChart = false
			installer.HelmWait = "0"
			Expect(installer.Install()).To(Succeed())
		})

		It("should be able to export the Lightbend credentials for a remote chart", func() {
			installer := lbc.DefaultInstaller()
			installer.AdditionalLBCArgs = []string{"--export-yaml=creds", "--version=1.1.0"}
			// jsravn: This is necessary to prevent leaking credentials in builds.
			installer.AdditionalHelmArgs = []string{"> /dev/null"}
			installer.LocalChart = false
			installer.HelmWait = "0"
			Expect(installer.Install()).To(Succeed())
		})

		It("should be able to export the console yaml for a local chart", func() {
			installer := lbc.DefaultInstaller()
			installer.AdditionalLBCArgs = []string{"--export-yaml=console"}
			installer.HelmWait = "0"
			Expect(installer.Install()).To(Succeed())
		})

		It("should be able to export the Lightbend credentials for a local chart", func() {
			installer := lbc.DefaultInstaller()
			installer.AdditionalLBCArgs = []string{"--export-yaml=creds"}
			// jsravn: This is necessary to prevent leaking credentials in builds.
			installer.AdditionalHelmArgs = []string{"> /dev/null"}
			installer.HelmWait = "0"
			Expect(installer.Install()).To(Succeed())
		})
	})

	Context("debug-dump", func() {
		It("should contain the pod logs", func() {
			// Install with default values, otherwise prior tests will cause problems.
			installer := lbc.DefaultInstaller()
			Expect(installer.Install()).To(Succeed())

			Expect(util.Cmd("/bin/bash", "-c", lbc.Path+" debug-dump --namespace="+args.ConsoleNamespace).
				Timeout(0).Run()).To(Succeed())

			dir, err := ioutil.TempDir("", "lbcpytest")
			defer os.RemoveAll(dir)
			if err != nil {
				panic(err)
			}
			Expect(util.Cmd("/bin/bash", "-c", "mv *.zip "+dir+"/").Run()).To(Succeed())
			files, err := ioutil.ReadDir(dir)
			if err != nil {
				panic(err)
			}

			Expect(files).To(HaveLen(1))
			zipFile := dir + "/" + files[0].Name()
			Expect(util.Cmd("/bin/bash", "-c", "cd "+dir+" && unzip "+zipFile).Run()).To(Succeed())

			matches, err := filepath.Glob(dir + "/console-backend*prometheus.log")
			if err != nil {
				panic(err)
			}
			Expect(matches).To(HaveLen(1), "should have found prometheus.log")
			promLogFile := matches[0]
			contents, err := ioutil.ReadFile(promLogFile)
			if err != nil {
				panic(err)
			}
			Expect(string(contents)).To(ContainSubstring("Completed loading of configuration file"),
				"%s should contain logs", promLogFile)
		})
	})
})
