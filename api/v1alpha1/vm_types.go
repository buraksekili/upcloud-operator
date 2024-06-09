// +kubebuilder:object:generate=true
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type CreateServerStorageDeviceSlice []CreateServerStorageDevice

// CreateServerStorageDevice represents a storage device for a CreateServerRequest
type CreateServerStorageDevice struct {
	Action    string `json:"action"`
	Address   string `json:"address,omitempty"`
	Encrypted int    `json:"encrypted,omitempty"`
	Storage   string `json:"storage"`
	Title     string `json:"title,omitempty"`
	// Storage size in gigabytes
	Size int    `json:"size,omitempty"`
	Tier string `json:"tier,omitempty"`
	Type string `json:"type,omitempty"`
}

type VMConfig struct {
	AvoidHost            int                            `json:"avoid_host,omitempty"`
	Host                 int                            `json:"host,omitempty"`
	BootOrder            string                         `json:"boot_order,omitempty"`
	CoreNumber           int                            `json:"core_number,omitempty"`
	Firewall             string                         `json:"firewall,omitempty"`
	Hostname             string                         `json:"hostname"`
	MemoryAmount         int                            `json:"memory_amount,omitempty"`
	Metadata             int                            `json:"metadata"`
	Plan                 string                         `json:"plan,omitempty"`
	ServerGroup          string                         `json:"server_group,omitempty"`
	SimpleBackup         string                         `json:"simple_backup,omitempty"`
	StorageDevices       CreateServerStorageDeviceSlice `json:"storage_devices"`
	TimeZone             string                         `json:"timezone,omitempty"`
	Title                string                         `json:"title"`
	UserData             string                         `json:"user_data,omitempty"`
	RemoteAccessEnabled  int                            `json:"remote_access_enabled,omitempty"`
	RemoteAccessType     string                         `json:"remote_access_type,omitempty"`
	RemoteAccessPassword string                         `json:"remote_access_password,omitempty"`
	Zone                 string                         `json:"zone"`
}

// ServerDetails represents details about a server
type ServerDetails struct {
	Details VMConfig `json:",inline"`
}

// VMSpec defines the desired state of VM
type VMSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Server *VMConfig `json:"server"`
}

type ServerState string

// VMStatus defines the observed state of VM
type VMStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	UpCloudVmHash string `json:"UpCloudVmHash,omitempty"`
	UUID          string `json:"UUID,omitempty"`
	State         string `json:"state"`
	Title         string `json:"title"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Hostname",type=string,JSONPath=`.spec.server.hostname`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Title",type=string,JSONPath=`.status.title`
// +kubebuilder:printcolumn:name="UUID",type=string,JSONPath=`.status.UUID`

// VM is the Schema for the vms API
type VM struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VMSpec   `json:"spec,omitempty"`
	Status VMStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VMList contains a list of VM
type VMList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VM `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VM{}, &VMList{})
}
