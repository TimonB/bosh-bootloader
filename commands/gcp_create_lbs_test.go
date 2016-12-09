package commands_test

import (
	"errors"

	"github.com/cloudfoundry/bosh-bootloader/commands"
	"github.com/cloudfoundry/bosh-bootloader/fakes"
	"github.com/cloudfoundry/bosh-bootloader/storage"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
)

const expectedTemplate = `variable "project_id" {
	type = "string"
}

variable "region" {
	type = "string"
}

variable "zone" {
	type = "string"
}

variable "env_id" {
	type = "string"
}

variable "credentials" {
	type = "string"
}

provider "google" {
	credentials = "${file("${var.credentials}")}"
	project = "${var.project_id}"
	region = "${var.region}"
}

output "external_ip" {
    value = "${google_compute_address.bosh-external-ip.address}"
}

output "network_name" {
    value = "${google_compute_network.bbl-network.name}"
}

output "subnetwork_name" {
    value = "${google_compute_subnetwork.bbl-subnet.name}"
}

output "bosh_open_tag_name" {
    value = "${google_compute_firewall.bosh-open.name}"
}

output "internal_tag_name" {
    value = "${google_compute_firewall.internal.name}"
}

output "director_address" {
	value = "https://${google_compute_address.bosh-external-ip.address}:25555"
}

resource "google_compute_network" "bbl-network" {
  name		 = "${var.env_id}-network"
}

resource "google_compute_subnetwork" "bbl-subnet" {
  name			= "${var.env_id}-subnet"
  ip_cidr_range = "10.0.0.0/16"
  network		= "${google_compute_network.bbl-network.self_link}"
}

resource "google_compute_address" "bosh-external-ip" {
  name = "${var.env_id}-bosh-external-ip"
}

resource "google_compute_firewall" "bosh-open" {
  name    = "${var.env_id}-bosh-open"
  network = "${google_compute_network.bbl-network.name}"

  source_ranges = ["0.0.0.0/0"]

  allow {
    protocol = "icmp"
  }

  allow {
    ports = ["22", "6868", "25555"]
    protocol = "tcp"
  }

  target_tags = ["${var.env_id}-bosh-open"]
}

resource "google_compute_firewall" "internal" {
  name    = "${var.env_id}-internal"
  network = "${google_compute_network.bbl-network.name}"

  allow {
    protocol = "icmp"
  }

  allow {
    protocol = "tcp"
  }

  allow {
    protocol = "udp"
  }

  source_tags = ["${var.env_id}-bosh-open","${var.env_id}-internal"]
}

output "concourse_target_pool" {
	value = "${google_compute_target_pool.target-pool.name}"
}

resource "google_compute_firewall" "firewall-concourse" {
  name    = "${var.env_id}-concourse-open"
  network = "${google_compute_network.bbl-network.name}"

  allow {
    protocol = "tcp"
    ports    = ["443", "2222"]
  }

  target_tags = ["concourse"]
}

resource "google_compute_address" "concourse-address" {
  name = "${var.env_id}-concourse"
}

resource "google_compute_http_health_check" "health-check" {
  name               = "${var.env_id}-concourse"
  request_path       = "/login"
  port               = 443
  check_interval_sec  = 30
  timeout_sec         = 5
  healthy_threshold   = 10
  unhealthy_threshold = 2
}

resource "google_compute_target_pool" "target-pool" {
  name = "${var.env_id}-concourse"

  health_checks = [
    "${google_compute_http_health_check.health-check.name}",
  ]
}

resource "google_compute_forwarding_rule" "ssh-forwarding-rule" {
  name        = "${var.env_id}-concourse-ssh"
  target      = "${google_compute_target_pool.target-pool.self_link}"
  port_range  = "2222"
  ip_protocol = "TCP"
  ip_address  = "${google_compute_address.concourse-address.address}"
}

resource "google_compute_forwarding_rule" "https-forwarding-rule" {
  name        = "${var.env_id}-concourse-https"
  target      = "${google_compute_target_pool.target-pool.self_link}"
  port_range  = "443"
  ip_protocol = "TCP"
  ip_address  = "${google_compute_address.concourse-address.address}"
}`

var _ = Describe("GCPCreateLBs", func() {
	var (
		cloudConfigGenerator *fakes.GCPCloudConfigGenerator
		terraformExecutor    *fakes.TerraformExecutor
		terraformOutputter   *fakes.TerraformOutputter
		boshClientProvider   *fakes.BOSHClientProvider
		boshClient           *fakes.BOSHClient
		zones                *fakes.Zones
		stateStore           *fakes.StateStore
		command              commands.GCPCreateLBs
	)

	BeforeEach(func() {
		terraformExecutor = &fakes.TerraformExecutor{}
		cloudConfigGenerator = &fakes.GCPCloudConfigGenerator{}
		terraformOutputter = &fakes.TerraformOutputter{}
		boshClientProvider = &fakes.BOSHClientProvider{}
		boshClient = &fakes.BOSHClient{}
		boshClientProvider.ClientCall.Returns.Client = boshClient
		zones = &fakes.Zones{}
		stateStore = &fakes.StateStore{}

		command = commands.NewGCPCreateLBs(terraformExecutor, terraformOutputter, cloudConfigGenerator, boshClientProvider, zones, stateStore)
	})

	AfterEach(func() {
		commands.ResetMarshal()
	})

	Describe("Execute", func() {
		It("creates and applies a concourse target pool", func() {
			err := command.Execute(commands.GCPCreateLBsConfig{
				LBType: "concourse",
			}, storage.State{
				IAAS:    "gcp",
				EnvID:   "some-env-id",
				TFState: "some-prev-tf-state",
				GCP: storage.GCP{
					ServiceAccountKey: "some-service-account-key",
					Zone:              "some-zone",
					Region:            "some-region",
				},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(terraformExecutor.ApplyCall.CallCount).To(Equal(1))
			Expect(terraformExecutor.ApplyCall.Receives.Credentials).To(Equal("some-service-account-key"))
			Expect(terraformExecutor.ApplyCall.Receives.EnvID).To(Equal("some-env-id"))
			Expect(terraformExecutor.ApplyCall.Receives.Zone).To(Equal("some-zone"))
			Expect(terraformExecutor.ApplyCall.Receives.Region).To(Equal("some-region"))
			Expect(terraformExecutor.ApplyCall.Receives.TFState).To(Equal("some-prev-tf-state"))
			Expect(terraformExecutor.ApplyCall.Receives.Template).To(Equal(expectedTemplate))
		})

		It("creates a cloud-config", func() {
			terraformOutputter.GetCall.Stub = func(output string) (string, error) {
				switch output {
				case "network_name":
					return "some-network-name", nil
				case "subnetwork_name":
					return "some-subnetwork-name", nil
				case "internal_tag_name":
					return "some-internal-tag", nil
				case "concourse_target_pool":
					return "env-id-concourse-target-pool", nil
				default:
					return "", nil
				}
			}

			zones.GetCall.Returns.Zones = []string{"region1", "region2"}

			err := command.Execute(commands.GCPCreateLBsConfig{
				LBType: "concourse",
			}, storage.State{
				IAAS: "gcp",
				GCP: storage.GCP{
					Region: "some-region",
				},
				BOSH: storage.BOSH{
					DirectorUsername: "some-director-username",
					DirectorPassword: "some-director-password",
					DirectorAddress:  "some-director-address",
				},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(zones.GetCall.CallCount).To(Equal(1))
			Expect(zones.GetCall.Receives.Region).To(Equal("some-region"))

			Expect(terraformOutputter.GetCall.CallCount).To(Equal(4))

			Expect(cloudConfigGenerator.GenerateCall.CallCount).To(Equal(1))
			Expect(cloudConfigGenerator.GenerateCall.Receives.CloudConfigInput.AZs).To(Equal([]string{"region1", "region2"}))
			Expect(cloudConfigGenerator.GenerateCall.Receives.CloudConfigInput.Tags).To(Equal([]string{"some-internal-tag"}))
			Expect(cloudConfigGenerator.GenerateCall.Receives.CloudConfigInput.NetworkName).To(Equal("some-network-name"))
			Expect(cloudConfigGenerator.GenerateCall.Receives.CloudConfigInput.SubnetworkName).To(Equal("some-subnetwork-name"))
			Expect(cloudConfigGenerator.GenerateCall.Receives.CloudConfigInput.LoadBalancer).To(Equal("env-id-concourse-target-pool"))
		})

		It("uploads a new cloud-config to the bosh director", func() {
			err := command.Execute(commands.GCPCreateLBsConfig{
				LBType: "concourse",
			}, storage.State{
				IAAS: "gcp",
				BOSH: storage.BOSH{
					DirectorUsername: "some-director-username",
					DirectorPassword: "some-director-password",
					DirectorAddress:  "some-director-address",
				},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(boshClientProvider.ClientCall.Receives.DirectorAddress).To(Equal("some-director-address"))
			Expect(boshClientProvider.ClientCall.Receives.DirectorUsername).To(Equal("some-director-username"))
			Expect(boshClientProvider.ClientCall.Receives.DirectorPassword).To(Equal("some-director-password"))

			Expect(boshClient.UpdateCloudConfigCall.CallCount).To(Equal(1))
		})

		Context("state manipulation", func() {
			It("saves the updated tfstate", func() {
				terraformExecutor.ApplyCall.Returns.TFState = "some-new-tfstate"
				err := command.Execute(commands.GCPCreateLBsConfig{
					LBType: "concourse",
				}, storage.State{
					IAAS:    "gcp",
					TFState: "some-old-tfstate",
					BOSH: storage.BOSH{
						DirectorUsername: "some-director-username",
						DirectorPassword: "some-director-password",
						DirectorAddress:  "some-director-address",
					},
				})
				Expect(err).NotTo(HaveOccurred())

				Expect(stateStore.SetCall.Receives.State.TFState).To(Equal("some-new-tfstate"))
			})
		})

		Context("failure cases", func() {
			It("returns an error when the lb type is not concourse", func() {
				err := command.Execute(commands.GCPCreateLBsConfig{
					LBType: "some-fake-lb",
				}, storage.State{IAAS: "gcp"})
				Expect(err).To(MatchError(`"some-fake-lb" is not a valid lb type, valid lb types are: concourse`))
			})

			It("returns an error when the terraform executor fails", func() {
				terraformExecutor.ApplyCall.Returns.Error = errors.New("failed to apply terraform")
				err := command.Execute(commands.GCPCreateLBsConfig{
					LBType: "concourse",
				}, storage.State{IAAS: "gcp"})

				Expect(err).To(MatchError("failed to apply terraform"))
			})

			It("returns an error when the state store fails to save the state", func() {
				stateStore.SetCall.Returns = []fakes.SetCallReturn{fakes.SetCallReturn{Error: errors.New("failed to save state")}}
				err := command.Execute(commands.GCPCreateLBsConfig{
					LBType: "concourse",
				}, storage.State{IAAS: "gcp"})

				Expect(err).To(MatchError("failed to save state"))
			})

			DescribeTable("returns an error when we fail to get an output", func(outputName string) {
				terraformOutputter.GetCall.Stub = func(output string) (string, error) {
					if output == outputName {
						return "", errors.New("failed to get output")
					}
					return "", nil
				}

				err := command.Execute(commands.GCPCreateLBsConfig{
					LBType: "concourse",
				}, storage.State{IAAS: "gcp"})
				Expect(err).To(MatchError("failed to get output"))
			},
				Entry("failed to get network_name", "network_name"),
				Entry("failed to get subnetwork_name", "subnetwork_name"),
				Entry("failed to get internal_tag_name", "internal_tag_name"),
				Entry("failed to get concourse_target_pool", "concourse_target_pool"),
			)

			It("returns an error when the cloud config fails to be generated", func() {
				cloudConfigGenerator.GenerateCall.Returns.Error = errors.New("failed to generate cloud config")

				err := command.Execute(commands.GCPCreateLBsConfig{
					LBType: "concourse",
				}, storage.State{IAAS: "gcp"})
				Expect(err).To(MatchError("failed to generate cloud config"))
			})

			It("returns an error when the cloud-config fails to marshal", func() {
				commands.SetMarshal(func(interface{}) ([]byte, error) {
					return []byte{}, errors.New("failed to marshal")
				})

				err := command.Execute(commands.GCPCreateLBsConfig{
					LBType: "concourse",
				}, storage.State{IAAS: "gcp"})
				Expect(err).To(MatchError("failed to marshal"))
			})

			It("returns an error when the cloud config fails to be updated", func() {
				boshClient.UpdateCloudConfigCall.Returns.Error = errors.New("failed to update cloud config")

				err := command.Execute(commands.GCPCreateLBsConfig{
					LBType: "concourse",
				}, storage.State{IAAS: "gcp"})
				Expect(err).To(MatchError("failed to update cloud config"))
			})

			It("returns an error when the iaas type is not gcp", func() {
				err := command.Execute(commands.GCPCreateLBsConfig{
					LBType: "concourse",
				}, storage.State{
					IAAS: "aws",
				})
				Expect(err).To(MatchError("iaas type must be gcp"))
			})
		})
	})
})