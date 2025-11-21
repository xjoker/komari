package clients

import (
	"sync"
	"time"

	pb "github.com/komari-monitor/komari/proto"
)

// ProfileCache stores static client profiles to avoid redundant storage
// Profiles are only updated when changes are detected
type ProfileCache struct {
	mu       sync.RWMutex
	profiles map[string]*CachedProfile
}

type CachedProfile struct {
	Profile   *pb.ClientProfile
	UpdatedAt time.Time
}

var (
	profileCache *ProfileCache
	once         sync.Once
)

// GetProfileCache returns the singleton profile cache instance
func GetProfileCache() *ProfileCache {
	once.Do(func() {
		profileCache = &ProfileCache{
			profiles: make(map[string]*CachedProfile),
		}
	})
	return profileCache
}

// Get retrieves a cached profile by UUID
func (pc *ProfileCache) Get(uuid string) (*pb.ClientProfile, bool) {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	cached, exists := pc.profiles[uuid]
	if !exists {
		return nil, false
	}
	return cached.Profile, true
}

// Set stores or updates a profile in the cache
func (pc *ProfileCache) Set(uuid string, profile *pb.ClientProfile) {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	pc.profiles[uuid] = &CachedProfile{
		Profile:   profile,
		UpdatedAt: time.Now(),
	}
}

// NeedsUpdate checks if a new profile differs from the cached version
// Returns true if the profile should be updated
func (pc *ProfileCache) NeedsUpdate(uuid string, newProfile *pb.ClientProfile) bool {
	cached, exists := pc.Get(uuid)
	if !exists {
		return true // No cached profile, needs update
	}

	// Compare key static fields to detect changes
	return cached.CpuName != newProfile.CpuName ||
		cached.CpuCores != newProfile.CpuCores ||
		cached.CpuArch != newProfile.CpuArch ||
		cached.RamTotal != newProfile.RamTotal ||
		cached.SwapTotal != newProfile.SwapTotal ||
		cached.DiskTotal != newProfile.DiskTotal ||
		cached.DiskPath != newProfile.DiskPath ||
		len(cached.Gpu) != len(newProfile.Gpu)
}

// Delete removes a profile from the cache
func (pc *ProfileCache) Delete(uuid string) {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	delete(pc.profiles, uuid)
}

// GetAll returns all cached profiles
func (pc *ProfileCache) GetAll() map[string]*pb.ClientProfile {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	result := make(map[string]*pb.ClientProfile, len(pc.profiles))
	for uuid, cached := range pc.profiles {
		result[uuid] = cached.Profile
	}
	return result
}

// Count returns the number of cached profiles
func (pc *ProfileCache) Count() int {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	return len(pc.profiles)
}

// Clear removes all cached profiles
func (pc *ProfileCache) Clear() {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	pc.profiles = make(map[string]*CachedProfile)
}
