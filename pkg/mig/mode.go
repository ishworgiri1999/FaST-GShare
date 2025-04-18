/*
 * Copyright (c) 2022, NVIDIA CORPORATION.  All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package mig

type Mode int

const (
	Disabled Mode = 0
	Enabled  Mode = 1
)

func (m Mode) String() string {
	switch m {
	case Disabled:
		return "Disabled"
	case Enabled:
		return "Enabled"
	}
	return "Unknown"
}

type Manager interface {
	IsMigCapable(gpu int) (bool, error)
	GetMigMode(gpu int) (Mode, error)
	SetMigMode(gpu int, mode Mode) error
	IsMigModeChangePending(gpu int) (bool, error)
}
