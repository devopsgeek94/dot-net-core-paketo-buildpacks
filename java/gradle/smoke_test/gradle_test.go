package gradle_test

import (
	"flag"
	"fmt"
	"github.com/paketo-buildpacks/samples/tests"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/paketo-buildpacks/occam"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	. "github.com/onsi/gomega"
	. "github.com/paketo-buildpacks/occam/matchers"
)

var builders tests.BuilderFlags

func init() {
	flag.Var(&builders, "name", "the name a builder to test with")
}
func TestGradle(t *testing.T) {
	Expect := NewWithT(t).Expect

	Expect(len(builders)).NotTo(Equal(0))

	SetDefaultEventuallyTimeout(60 * time.Second)

	suite := spec.New("Java - Gradle", spec.Parallel(), spec.Report(report.Terminal{}))
	for _, builder := range builders {
		suite(fmt.Sprintf("Gradle with %s builder", builder), testGradleWithBuilder(builder), spec.Sequential())
	}
	suite.Run(t)
}

func testGradleWithBuilder(builder string) func(*testing.T, spec.G, spec.S) {
	return func(t *testing.T, context spec.G, it spec.S) {
		var (
			Expect     = NewWithT(t).Expect
			Eventually = NewWithT(t).Eventually

			pack   occam.Pack
			docker occam.Docker
		)

		it.Before(func() {
			pack = occam.NewPack().WithVerbose().WithNoColor()
			docker = occam.NewDocker()
		})

		context("detects a Java app", func() {
			var (
				image     occam.Image
				container occam.Container

				name   string
				source string
			)

			it.Before(func() {
				var err error
				name, err = occam.RandomName()
				Expect(err).NotTo(HaveOccurred())
			})

			it.After(func() {
				err := docker.Container.Remove.Execute(container.ID)
				if err != nil {
					Expect(err).To(MatchError("failed to remove docker container: exit status 1: Container name cannot be empty"))
				} else {
					Expect(err).ToNot(HaveOccurred())
				}

				Expect(docker.Volume.Remove.Execute(occam.CacheVolumeNames(name))).To(Succeed())

				err = docker.Image.Remove.Execute(image.ID)
				if err != nil {
					Expect(err).To(MatchError(ContainSubstring("failed to remove docker image: exit status 1: Error")))
				} else {
					Expect(err).ToNot(HaveOccurred())
				}

				Expect(os.RemoveAll(source)).To(Succeed())
			})

			context("app uses gradle", func() {
				it("builds successfully", func() {
					var err error
					source, err = occam.Source(filepath.Join("../"))
					Expect(err).NotTo(HaveOccurred())

					var logs fmt.Stringer
					image, logs, err = pack.Build.
						WithPullPolicy("never").
						WithEnv(map[string]string{
							"BP_JVM_VERSION":           "17",
							"BP_GRADLE_BUILT_ARTIFACT": "build/libs/*-SNAPSHOT.jar",
							"SERVICE_BINDING_ROOT":     "/bindings",
						}).
						WithBuilder(builder).
						WithGID("123").
						WithVolumes(fmt.Sprintf("%s:/bindings/gradle-wrapper", filepath.Join(source, "bindings/gradle-wrapper"))).
						Execute(name, source)
					Expect(err).ToNot(HaveOccurred(), logs.String)

					Expect(logs).To(ContainLines(ContainSubstring("Paketo Buildpack for CA Certificates")))
					Expect(logs).To(ContainLines(ContainSubstring("Paketo Buildpack for BellSoft Liberica")))
					Expect(logs).To(ContainLines(ContainSubstring("Paketo Buildpack for Gradle")))
					Expect(logs).To(ContainLines(ContainSubstring("Paketo Buildpack for Executable JAR")))

					container, err = docker.Container.Run.
						WithPublish("8080").
						Execute(image.ID)
					Expect(err).NotTo(HaveOccurred())

					Eventually(container).Should(Serve(ContainSubstring("UP")).OnPort(8080).WithEndpoint("/actuator/health"))
				})
			})
		})
	}
}
